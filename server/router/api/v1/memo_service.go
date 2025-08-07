package v1

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lithammer/shortuuid/v4"
	"github.com/pkg/errors"
	"github.com/usememos/gomark/ast"
	"github.com/usememos/gomark/parser"
	"github.com/usememos/gomark/parser/tokenizer"
	"github.com/usememos/gomark/renderer"
	"github.com/usememos/gomark/restore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/usememos/memos/plugin/webhook"
	v1pb "github.com/usememos/memos/proto/gen/api/v1"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/server/runner/memopayload"
	"github.com/usememos/memos/store"
)

func (s *APIV1Service) CreateMemo(ctx context.Context, request *v1pb.CreateMemoRequest) (*v1pb.Memo, error) {
	user, err := s.GetCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user")
	}

	create := &store.Memo{
		UID:        shortuuid.New(),
		CreatorID:  user.ID,
		Content:    request.Memo.Content,
		Visibility: convertVisibilityToStore(request.Memo.Visibility),
	}
	workspaceMemoRelatedSetting, err := s.Store.GetWorkspaceMemoRelatedSetting(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get workspace memo related setting")
	}
	if workspaceMemoRelatedSetting.DisallowPublicVisibility && create.Visibility == store.Public {
		return nil, status.Errorf(codes.PermissionDenied, "disable public memos system setting is enabled")
	}
	contentLengthLimit, err := s.getContentLengthLimit(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get content length limit")
	}
	if len(create.Content) > contentLengthLimit {
		return nil, status.Errorf(codes.InvalidArgument, "content too long (max %d characters)", contentLengthLimit)
	}
	if err := memopayload.RebuildMemoPayload(create); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to rebuild memo payload: %v", err)
	}
	if request.Memo.Location != nil {
		create.Payload.Location = convertLocationToStore(request.Memo.Location)
	}

	memo, err := s.Store.CreateMemo(ctx, create)
	if err != nil {
		return nil, err
	}
	if len(request.Memo.Attachments) > 0 {
		_, err := s.SetMemoAttachments(ctx, &v1pb.SetMemoAttachmentsRequest{
			Name:        fmt.Sprintf("%s%s", MemoNamePrefix, memo.UID),
			Attachments: request.Memo.Attachments,
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed to set memo attachments")
		}
	}
	if len(request.Memo.Relations) > 0 {
		_, err := s.SetMemoRelations(ctx, &v1pb.SetMemoRelationsRequest{
			Name:      fmt.Sprintf("%s%s", MemoNamePrefix, memo.UID),
			Relations: request.Memo.Relations,
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed to set memo relations")
		}
	}

	memoMessage, err := s.convertMemoFromStore(ctx, memo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert memo")
	}
	// Try to dispatch webhook when memo is created.
	if err := s.DispatchMemoCreatedWebhook(ctx, memoMessage); err != nil {
		slog.Warn("Failed to dispatch memo created webhook", slog.Any("err", err))
	}

	return memoMessage, nil
}

func (s *APIV1Service) ListMemos(ctx context.Context, request *v1pb.ListMemosRequest) (*v1pb.ListMemosResponse, error) {
	memoFind := &store.FindMemo{
		// Exclude comments by default.
		ExcludeComments: true,
	}
	if request.State == v1pb.State_ARCHIVED {
		state := store.Archived
		memoFind.RowStatus = &state
	} else {
		state := store.Normal
		memoFind.RowStatus = &state
	}

	// Parse order_by field (replaces the old sort and direction fields)
	if request.OrderBy != "" {
		if err := s.parseMemoOrderBy(request.OrderBy, memoFind); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid order_by: %v", err)
		}
	} else {
		// Default ordering by display_time desc
		memoFind.OrderByTimeAsc = false
	}

	if request.Filter != "" {
		if err := s.validateFilter(ctx, request.Filter); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid filter: %v", err)
		}
		memoFind.Filters = append(memoFind.Filters, request.Filter)
	}

	currentUser, err := s.GetCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user")
	}
	if currentUser == nil {
		memoFind.VisibilityList = []store.Visibility{store.Public}
	} else {
		if memoFind.CreatorID == nil {
			filter := fmt.Sprintf(`creator_id == %d || visibility in ["PUBLIC", "PROTECTED"]`, currentUser.ID)
			memoFind.Filters = append(memoFind.Filters, filter)
		} else if *memoFind.CreatorID != currentUser.ID {
			memoFind.VisibilityList = []store.Visibility{store.Public, store.Protected}
		}
	}

	workspaceMemoRelatedSetting, err := s.Store.GetWorkspaceMemoRelatedSetting(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get workspace memo related setting")
	}
	if workspaceMemoRelatedSetting.DisplayWithUpdateTime {
		memoFind.OrderByUpdatedTs = true
	}

	var limit, offset int
	if request.PageToken != "" {
		var pageToken v1pb.PageToken
		if err := unmarshalPageToken(request.PageToken, &pageToken); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid page token: %v", err)
		}
		limit = int(pageToken.Limit)
		offset = int(pageToken.Offset)
	} else {
		limit = int(request.PageSize)
	}
	if limit <= 0 {
		limit = DefaultPageSize
	}
	limitPlusOne := limit + 1
	memoFind.Limit = &limitPlusOne
	memoFind.Offset = &offset
	memos, err := s.Store.ListMemos(ctx, memoFind)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list memos: %v", err)
	}

	memoMessages := []*v1pb.Memo{}
	nextPageToken := ""
	if len(memos) == limitPlusOne {
		memos = memos[:limit]
		nextPageToken, err = getPageToken(limit, offset+limit)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get next page token, error: %v", err)
		}
	}
	for _, memo := range memos {
		memoMessage, err := s.convertMemoFromStore(ctx, memo)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert memo")
		}
		memoMessages = append(memoMessages, memoMessage)
	}

	response := &v1pb.ListMemosResponse{
		Memos:         memoMessages,
		NextPageToken: nextPageToken,
	}
	return response, nil
}

func (s *APIV1Service) GetMemo(ctx context.Context, request *v1pb.GetMemoRequest) (*v1pb.Memo, error) {
	memoUID, err := ExtractMemoUIDFromName(request.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid memo name: %v", err)
	}
	memo, err := s.Store.GetMemo(ctx, &store.FindMemo{
		UID: &memoUID,
	})
	if err != nil {
		return nil, err
	}
	if memo == nil {
		return nil, status.Errorf(codes.NotFound, "memo not found")
	}
	if memo.Visibility != store.Public {
		user, err := s.GetCurrentUser(ctx)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get user")
		}
		if user == nil {
			return nil, status.Errorf(codes.PermissionDenied, "permission denied")
		}
		if memo.Visibility == store.Private && memo.CreatorID != user.ID {
			return nil, status.Errorf(codes.PermissionDenied, "permission denied")
		}
	}

	memoMessage, err := s.convertMemoFromStore(ctx, memo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert memo")
	}
	return memoMessage, nil
}

func (s *APIV1Service) UpdateMemo(ctx context.Context, request *v1pb.UpdateMemoRequest) (*v1pb.Memo, error) {
	memoUID, err := ExtractMemoUIDFromName(request.Memo.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid memo name: %v", err)
	}
	if request.UpdateMask == nil || len(request.UpdateMask.Paths) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "update mask is required")
	}

	memo, err := s.Store.GetMemo(ctx, &store.FindMemo{UID: &memoUID})
	if err != nil {
		return nil, err
	}
	if memo == nil {
		return nil, status.Errorf(codes.NotFound, "memo not found")
	}

	user, err := s.GetCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user")
	}
	// Only the creator or admin can update the memo.
	if memo.CreatorID != user.ID && !isSuperUser(user) {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}

	update := &store.UpdateMemo{
		ID: memo.ID,
	}
	for _, path := range request.UpdateMask.Paths {
		if path == "content" {
			contentLengthLimit, err := s.getContentLengthLimit(ctx)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "failed to get content length limit")
			}
			if len(request.Memo.Content) > contentLengthLimit {
				return nil, status.Errorf(codes.InvalidArgument, "content too long (max %d characters)", contentLengthLimit)
			}
			memo.Content = request.Memo.Content
			if err := memopayload.RebuildMemoPayload(memo); err != nil {
				return nil, status.Errorf(codes.Internal, "failed to rebuild memo payload: %v", err)
			}
			update.Content = &memo.Content
			update.Payload = memo.Payload
		} else if path == "visibility" {
			workspaceMemoRelatedSetting, err := s.Store.GetWorkspaceMemoRelatedSetting(ctx)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "failed to get workspace memo related setting")
			}
			visibility := convertVisibilityToStore(request.Memo.Visibility)
			if workspaceMemoRelatedSetting.DisallowPublicVisibility && visibility == store.Public {
				return nil, status.Errorf(codes.PermissionDenied, "disable public memos system setting is enabled")
			}
			update.Visibility = &visibility
		} else if path == "pinned" {
			update.Pinned = &request.Memo.Pinned
		} else if path == "state" {
			rowStatus := convertStateToStore(request.Memo.State)
			update.RowStatus = &rowStatus
		} else if path == "create_time" {
			createdTs := request.Memo.CreateTime.AsTime().Unix()
			update.CreatedTs = &createdTs
		} else if path == "update_time" {
			updatedTs := time.Now().Unix()
			if request.Memo.UpdateTime != nil {
				updatedTs = request.Memo.UpdateTime.AsTime().Unix()
			}
			update.UpdatedTs = &updatedTs
		} else if path == "display_time" {
			displayTs := request.Memo.DisplayTime.AsTime().Unix()
			memoRelatedSetting, err := s.Store.GetWorkspaceMemoRelatedSetting(ctx)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "failed to get workspace memo related setting")
			}
			if memoRelatedSetting.DisplayWithUpdateTime {
				update.UpdatedTs = &displayTs
			} else {
				update.CreatedTs = &displayTs
			}
		} else if path == "location" {
			payload := memo.Payload
			payload.Location = convertLocationToStore(request.Memo.Location)
			update.Payload = payload
		} else if path == "attachments" {
			_, err := s.SetMemoAttachments(ctx, &v1pb.SetMemoAttachmentsRequest{
				Name:        request.Memo.Name,
				Attachments: request.Memo.Attachments,
			})
			if err != nil {
				return nil, errors.Wrap(err, "failed to set memo attachments")
			}
		} else if path == "relations" {
			_, err := s.SetMemoRelations(ctx, &v1pb.SetMemoRelationsRequest{
				Name:      request.Memo.Name,
				Relations: request.Memo.Relations,
			})
			if err != nil {
				return nil, errors.Wrap(err, "failed to set memo relations")
			}
		}
	}

	if err = s.Store.UpdateMemo(ctx, update); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update memo")
	}

	memo, err = s.Store.GetMemo(ctx, &store.FindMemo{
		ID: &memo.ID,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get memo")
	}
	memoMessage, err := s.convertMemoFromStore(ctx, memo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert memo")
	}
	// Try to dispatch webhook when memo is updated.
	if err := s.DispatchMemoUpdatedWebhook(ctx, memoMessage); err != nil {
		slog.Warn("Failed to dispatch memo updated webhook", slog.Any("err", err))
	}

	return memoMessage, nil
}

func (s *APIV1Service) DeleteMemo(ctx context.Context, request *v1pb.DeleteMemoRequest) (*emptypb.Empty, error) {
	memoUID, err := ExtractMemoUIDFromName(request.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid memo name: %v", err)
	}
	memo, err := s.Store.GetMemo(ctx, &store.FindMemo{
		UID: &memoUID,
	})
	if err != nil {
		return nil, err
	}
	if memo == nil {
		return nil, status.Errorf(codes.NotFound, "memo not found")
	}

	user, err := s.GetCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user")
	}
	// Only the creator or admin can update the memo.
	if memo.CreatorID != user.ID && !isSuperUser(user) {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}

	if memoMessage, err := s.convertMemoFromStore(ctx, memo); err == nil {
		// Try to dispatch webhook when memo is deleted.
		if err := s.DispatchMemoDeletedWebhook(ctx, memoMessage); err != nil {
			slog.Warn("Failed to dispatch memo deleted webhook", slog.Any("err", err))
		}
	}

	if err = s.Store.DeleteMemo(ctx, &store.DeleteMemo{ID: memo.ID}); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete memo")
	}

	// Delete memo relation
	if err := s.Store.DeleteMemoRelation(ctx, &store.DeleteMemoRelation{MemoID: &memo.ID}); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete memo relations")
	}

	// Delete related attachments.
	attachments, err := s.Store.ListAttachments(ctx, &store.FindAttachment{MemoID: &memo.ID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list attachments")
	}
	for _, attachment := range attachments {
		if err := s.Store.DeleteAttachment(ctx, &store.DeleteAttachment{ID: attachment.ID}); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to delete attachment")
		}
	}

	// Delete memo comments
	commentType := store.MemoRelationComment
	relations, err := s.Store.ListMemoRelations(ctx, &store.FindMemoRelation{RelatedMemoID: &memo.ID, Type: &commentType})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list memo comments")
	}
	for _, relation := range relations {
		if err := s.Store.DeleteMemo(ctx, &store.DeleteMemo{ID: relation.MemoID}); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to delete memo comment")
		}
	}

	// Delete memo references
	referenceType := store.MemoRelationReference
	if err := s.Store.DeleteMemoRelation(ctx, &store.DeleteMemoRelation{RelatedMemoID: &memo.ID, Type: &referenceType}); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete memo references")
	}

	return &emptypb.Empty{}, nil
}

func (s *APIV1Service) CreateMemoComment(ctx context.Context, request *v1pb.CreateMemoCommentRequest) (*v1pb.Memo, error) {
	memoUID, err := ExtractMemoUIDFromName(request.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid memo name: %v", err)
	}
	relatedMemo, err := s.Store.GetMemo(ctx, &store.FindMemo{UID: &memoUID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get memo")
	}

	// Create the memo comment first.
	memoComment, err := s.CreateMemo(ctx, &v1pb.CreateMemoRequest{Memo: request.Comment})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create memo")
	}
	memoUID, err = ExtractMemoUIDFromName(memoComment.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid memo name: %v", err)
	}
	memo, err := s.Store.GetMemo(ctx, &store.FindMemo{UID: &memoUID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get memo")
	}

	// Build the relation between the comment memo and the original memo.
	_, err = s.Store.UpsertMemoRelation(ctx, &store.MemoRelation{
		MemoID:        memo.ID,
		RelatedMemoID: relatedMemo.ID,
		Type:          store.MemoRelationComment,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create memo relation")
	}
	creatorID, err := ExtractUserIDFromName(memoComment.Creator)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid memo creator")
	}
	if memoComment.Visibility != v1pb.Visibility_PRIVATE && creatorID != relatedMemo.CreatorID {
		activity, err := s.Store.CreateActivity(ctx, &store.Activity{
			CreatorID: creatorID,
			Type:      store.ActivityTypeMemoComment,
			Level:     store.ActivityLevelInfo,
			Payload: &storepb.ActivityPayload{
				MemoComment: &storepb.ActivityMemoCommentPayload{
					MemoId:        memo.ID,
					RelatedMemoId: relatedMemo.ID,
				},
			},
		})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to create activity")
		}
		if _, err := s.Store.CreateInbox(ctx, &store.Inbox{
			SenderID:   creatorID,
			ReceiverID: relatedMemo.CreatorID,
			Status:     store.UNREAD,
			Message: &storepb.InboxMessage{
				Type:       storepb.InboxMessage_MEMO_COMMENT,
				ActivityId: &activity.ID,
			},
		}); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to create inbox")
		}
	}

	return memoComment, nil
}

func (s *APIV1Service) ListMemoComments(ctx context.Context, request *v1pb.ListMemoCommentsRequest) (*v1pb.ListMemoCommentsResponse, error) {
	memoUID, err := ExtractMemoUIDFromName(request.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid memo name: %v", err)
	}
	memo, err := s.Store.GetMemo(ctx, &store.FindMemo{UID: &memoUID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get memo")
	}

	currentUser, err := s.GetCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user")
	}
	var memoFilter string
	if currentUser == nil {
		memoFilter = `visibility == "PUBLIC"`
	} else {
		memoFilter = fmt.Sprintf(`creator_id == %d || visibility in ["PUBLIC", "PROTECTED"]`, currentUser.ID)
	}
	memoRelationComment := store.MemoRelationComment
	memoRelations, err := s.Store.ListMemoRelations(ctx, &store.FindMemoRelation{
		RelatedMemoID: &memo.ID,
		Type:          &memoRelationComment,
		MemoFilter:    &memoFilter,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list memo relations")
	}

	var memos []*v1pb.Memo
	for _, memoRelation := range memoRelations {
		memo, err := s.Store.GetMemo(ctx, &store.FindMemo{
			ID: &memoRelation.MemoID,
		})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get memo")
		}
		if memo != nil {
			memoMessage, err := s.convertMemoFromStore(ctx, memo)
			if err != nil {
				return nil, errors.Wrap(err, "failed to convert memo")
			}
			memos = append(memos, memoMessage)
		}
	}

	response := &v1pb.ListMemoCommentsResponse{
		Memos: memos,
	}
	return response, nil
}

func (s *APIV1Service) RenameMemoTag(ctx context.Context, request *v1pb.RenameMemoTagRequest) (*emptypb.Empty, error) {
	user, err := s.GetCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user")
	}

	// Reject global operations (memos/-) - use TagService.RenameTag instead
	if request.Parent == "memos/-" {
		return nil, status.Errorf(codes.InvalidArgument, 
			"Global tag operations are no longer supported. Use TagService.RenameTag for global tag renaming.")
	}

	// Extract memo UID from parent
	memoUID, err := ExtractMemoUIDFromName(request.Parent)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid memo name: %v", err)
	}

	memoFind := &store.FindMemo{
		CreatorID:       &user.ID,
		UID:             &memoUID,
		Filters:         []string{fmt.Sprintf("tag in [\"%s\"]", request.OldTag)},
		ExcludeComments: true,
	}

	memos, err := s.Store.ListMemos(ctx, memoFind)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list memos")
	}

	for _, memo := range memos {
		nodes, err := parser.Parse(tokenizer.Tokenize(memo.Content))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to parse memo: %v", err)
		}
		memopayload.TraverseASTNodes(nodes, func(node ast.Node) {
			if tag, ok := node.(*ast.Tag); ok && tag.Content == request.OldTag {
				tag.Content = request.NewTag
			}
		})
		memo.Content = restore.Restore(nodes)
		if err := memopayload.RebuildMemoPayload(memo); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to rebuild memo payload: %v", err)
		}
		if err := s.Store.UpdateMemo(ctx, &store.UpdateMemo{
			ID:      memo.ID,
			Content: &memo.Content,
			Payload: memo.Payload,
		}); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to update memo: %v", err)
		}
	}

	return &emptypb.Empty{}, nil
}

func (s *APIV1Service) DeleteMemoTag(ctx context.Context, request *v1pb.DeleteMemoTagRequest) (*emptypb.Empty, error) {
	user, err := s.GetCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user")
	}

	// Reject global operations (memos/-) - use TagService.DeleteTag instead
	if request.Parent == "memos/-" {
		return nil, status.Errorf(codes.InvalidArgument, 
			"Global tag operations are no longer supported. Use TagService.DeleteTag to remove tags globally, or MemoService.BatchDeleteMemosByTag to delete memos.")
	}

	// Extract memo UID from parent
	memoUID, err := ExtractMemoUIDFromName(request.Parent)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid memo name: %v", err)
	}

	memoFind := &store.FindMemo{
		CreatorID:       &user.ID,
		UID:             &memoUID,
		Filters:         []string{fmt.Sprintf("tag in [\"%s\"]", request.Tag)},
		ExcludeComments: true,
	}

	memos, err := s.Store.ListMemos(ctx, memoFind)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list memos")
	}

	// Remove the tag from the specific memo's content
	for _, memo := range memos {
		// Parse memo content and remove the specified tag
		nodes, err := parser.Parse(tokenizer.Tokenize(memo.Content))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to parse memo: %v", err)
		}
		
		// Remove the tag from AST using recursive traversal
		nodes = removeTagFromNodes(nodes, request.Tag)
		
		// Reconstruct content and update memo
		memo.Content = restore.Restore(nodes)
		if err := memopayload.RebuildMemoPayload(memo); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to rebuild memo payload: %v", err)
		}
		if err := s.Store.UpdateMemo(ctx, &store.UpdateMemo{
			ID:      memo.ID,
			Content: &memo.Content,
			Payload: memo.Payload,
		}); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to update memo: %v", err)
		}
	}

	return &emptypb.Empty{}, nil
}

// BatchDeleteMemosByTag deletes all memos containing a specific tag
func (s *APIV1Service) BatchDeleteMemosByTag(ctx context.Context, request *v1pb.BatchDeleteMemosByTagRequest) (*v1pb.BatchDeleteMemosByTagResponse, error) {
	user, err := s.GetCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user")
	}

	if request.TagPath == "" {
		return nil, status.Errorf(codes.InvalidArgument, "tag_path is required")
	}

	// Build tag filter for query
	var tagFilters []string
	if request.IncludeChildren {
		// Include all tags that start with the specified path
		tagFilters = append(tagFilters, fmt.Sprintf("tag starts_with [\"%s\"]", request.TagPath))
	} else {
		// Only exact match
		tagFilters = append(tagFilters, fmt.Sprintf("tag in [\"%s\"]", request.TagPath))
	}

	// Find all memos with the specified tag(s)
	memos, err := s.Store.ListMemos(ctx, &store.FindMemo{
		CreatorID:       &user.ID,
		Filters:         tagFilters,
		ExcludeComments: true,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list memos: %v", err)
	}

	// If dry_run is true, just return what would be deleted
	if request.DryRun {
		deletedMemoIDs := make([]string, len(memos))
		affectedTagPaths := []string{request.TagPath}
		
		for i, memo := range memos {
			deletedMemoIDs[i] = memo.UID
		}
		
		// If including children, add all child tag paths found
		if request.IncludeChildren {
			tagMap := make(map[string]bool)
			for _, memo := range memos {
				if memo.Payload != nil {
					for _, tag := range memo.Payload.Tags {
						if strings.HasPrefix(tag.Name, request.TagPath) {
							tagMap[tag.Name] = true
						}
					}
				}
			}
			for tagPath := range tagMap {
				if tagPath != request.TagPath {
					affectedTagPaths = append(affectedTagPaths, tagPath)
				}
			}
		}

		return &v1pb.BatchDeleteMemosByTagResponse{
			DeletedMemoIds:     deletedMemoIDs,
			DeletedCount:       int32(len(deletedMemoIDs)),
			AffectedTagPaths:   affectedTagPaths,
		}, nil
	}

	// Actually delete the memos
	deletedMemoIDs := make([]string, 0, len(memos))
	affectedTagPaths := make(map[string]bool)
	affectedTagPaths[request.TagPath] = true

	for _, memo := range memos {
		// Collect affected tag paths before deletion
		if memo.Payload != nil {
			for _, tag := range memo.Payload.Tags {
				if request.IncludeChildren && strings.HasPrefix(tag.Name, request.TagPath) {
					affectedTagPaths[tag.Name] = true
				} else if tag.Name == request.TagPath {
					affectedTagPaths[tag.Name] = true
				}
			}
		}

		// Delete the memo
		err := s.Store.DeleteMemo(ctx, &store.DeleteMemo{ID: memo.ID})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to delete memo %s: %v", memo.UID, err)
		}
		deletedMemoIDs = append(deletedMemoIDs, memo.UID)
	}

	// Convert map to slice
	affectedTagPathsSlice := make([]string, 0, len(affectedTagPaths))
	for tagPath := range affectedTagPaths {
		affectedTagPathsSlice = append(affectedTagPathsSlice, tagPath)
	}

	return &v1pb.BatchDeleteMemosByTagResponse{
		DeletedMemoIds:   deletedMemoIDs,
		DeletedCount:     int32(len(deletedMemoIDs)),
		AffectedTagPaths: affectedTagPathsSlice,
	}, nil
}

func (s *APIV1Service) getContentLengthLimit(ctx context.Context) (int, error) {
	workspaceMemoRelatedSetting, err := s.Store.GetWorkspaceMemoRelatedSetting(ctx)
	if err != nil {
		return 0, status.Errorf(codes.Internal, "failed to get workspace memo related setting")
	}
	return int(workspaceMemoRelatedSetting.ContentLengthLimit), nil
}

// DispatchMemoCreatedWebhook dispatches webhook when memo is created.
func (s *APIV1Service) DispatchMemoCreatedWebhook(ctx context.Context, memo *v1pb.Memo) error {
	return s.dispatchMemoRelatedWebhook(ctx, memo, "memos.memo.created")
}

// DispatchMemoUpdatedWebhook dispatches webhook when memo is updated.
func (s *APIV1Service) DispatchMemoUpdatedWebhook(ctx context.Context, memo *v1pb.Memo) error {
	return s.dispatchMemoRelatedWebhook(ctx, memo, "memos.memo.updated")
}

// DispatchMemoDeletedWebhook dispatches webhook when memo is deleted.
func (s *APIV1Service) DispatchMemoDeletedWebhook(ctx context.Context, memo *v1pb.Memo) error {
	return s.dispatchMemoRelatedWebhook(ctx, memo, "memos.memo.deleted")
}

func (s *APIV1Service) dispatchMemoRelatedWebhook(ctx context.Context, memo *v1pb.Memo, activityType string) error {
	creatorID, err := ExtractUserIDFromName(memo.Creator)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid memo creator")
	}
	webhooks, err := s.Store.GetUserWebhooks(ctx, creatorID)
	if err != nil {
		return err
	}
	for _, hook := range webhooks {
		payload, err := convertMemoToWebhookPayload(memo)
		if err != nil {
			return errors.Wrap(err, "failed to convert memo to webhook payload")
		}
		payload.ActivityType = activityType
		payload.URL = hook.Url

		// Use asynchronous webhook dispatch
		webhook.PostAsync(payload)
	}
	return nil
}

func convertMemoToWebhookPayload(memo *v1pb.Memo) (*webhook.WebhookRequestPayload, error) {
	creatorID, err := ExtractUserIDFromName(memo.Creator)
	if err != nil {
		return nil, errors.Wrap(err, "invalid memo creator")
	}
	return &webhook.WebhookRequestPayload{
		Creator: fmt.Sprintf("%s%d", UserNamePrefix, creatorID),
		Memo:    memo,
	}, nil
}

func getMemoContentSnippet(content string) (string, error) {
	nodes, err := parser.Parse(tokenizer.Tokenize(content))
	if err != nil {
		return "", errors.Wrap(err, "failed to parse content")
	}

	plainText := renderer.NewStringRenderer().Render(nodes)
	if len(plainText) > 64 {
		return substring(plainText, 64) + "...", nil
	}
	return plainText, nil
}

func substring(s string, length int) string {
	if length <= 0 {
		return ""
	}

	runeCount := 0
	byteIndex := 0
	for byteIndex < len(s) {
		_, size := utf8.DecodeRuneInString(s[byteIndex:])
		byteIndex += size
		runeCount++
		if runeCount == length {
			break
		}
	}

	return s[:byteIndex]
}

// parseMemoOrderBy parses the order_by field and sets the appropriate ordering in memoFind.
func (*APIV1Service) parseMemoOrderBy(orderBy string, memoFind *store.FindMemo) error {
	// Parse order_by field like "display_time desc" or "create_time asc"
	parts := strings.Fields(strings.TrimSpace(orderBy))
	if len(parts) == 0 {
		return errors.New("empty order_by")
	}

	field := parts[0]
	direction := "desc" // default
	if len(parts) > 1 {
		direction = strings.ToLower(parts[1])
		if direction != "asc" && direction != "desc" {
			return errors.Errorf("invalid order direction: %s, must be 'asc' or 'desc'", parts[1])
		}
	}

	switch field {
	case "display_time":
		memoFind.OrderByTimeAsc = direction == "asc"
	case "create_time":
		memoFind.OrderByTimeAsc = direction == "asc"
	case "update_time":
		memoFind.OrderByUpdatedTs = true
		memoFind.OrderByTimeAsc = direction == "asc"
	case "name":
		// For ordering by memo name/id - not commonly used but supported
		memoFind.OrderByTimeAsc = direction == "asc"
	default:
		return errors.Errorf("unsupported order field: %s, supported fields are: display_time, create_time, update_time, name", field)
	}

	return nil
}

// removeTagFromNodes recursively traverses AST nodes and removes the specified tag
func removeTagFromNodes(nodes []ast.Node, tagToRemove string) []ast.Node {
	result := make([]ast.Node, 0, len(nodes))
	
	for _, node := range nodes {
		switch n := node.(type) {
		case *ast.Tag:
			// If this tag matches the one we want to remove, skip it
			if n.Content != tagToRemove {
				result = append(result, node)
			}
		case *ast.Paragraph:
			// Tags are often within paragraphs, recurse into children
			n.Children = removeTagFromNodes(n.Children, tagToRemove)
			result = append(result, node)
		case *ast.Heading:
			// Tags can also be in headings
			n.Children = removeTagFromNodes(n.Children, tagToRemove)
			result = append(result, node)
		case *ast.Blockquote:
			// Tags can be in blockquotes
			n.Children = removeTagFromNodes(n.Children, tagToRemove)
			result = append(result, node)
		case *ast.List:
			// Tags can be in lists
			n.Children = removeTagFromNodes(n.Children, tagToRemove)
			result = append(result, node)
		case *ast.OrderedListItem:
			// Tags can be in ordered list items
			n.Children = removeTagFromNodes(n.Children, tagToRemove)
			result = append(result, node)
		case *ast.UnorderedListItem:
			// Tags can be in unordered list items
			n.Children = removeTagFromNodes(n.Children, tagToRemove)
			result = append(result, node)
		case *ast.TaskListItem:
			// Tags can be in task list items
			n.Children = removeTagFromNodes(n.Children, tagToRemove)
			result = append(result, node)
		case *ast.Bold:
			// Tags can be in bold text
			n.Children = removeTagFromNodes(n.Children, tagToRemove)
			result = append(result, node)
		case *ast.Italic:
			// Tags can be in italic text
			n.Children = removeTagFromNodes(n.Children, tagToRemove)
			result = append(result, node)
		default:
			// For all other node types, just include them
			result = append(result, node)
		}
	}
	
	return result
}
