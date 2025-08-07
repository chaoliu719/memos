package v1

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1pb "github.com/usememos/memos/proto/gen/api/v1"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
)

// ListTags lists all tags with optional filtering
func (s *APIV1Service) ListTags(ctx context.Context, request *v1pb.ListTagsRequest) (*v1pb.ListTagsResponse, error) {
	user, err := s.GetCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user")
	}

	// Get all memos for the current user
	memos, err := s.Store.ListMemos(ctx, &store.FindMemo{
		CreatorID:       &user.ID,
		ExcludeComments: true,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list memos: %v", err)
	}

	// Aggregate tags from all memos
	tagMap, err := s.aggregateTagsFromMemos(memos, user.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to aggregate tags: %v", err)
	}

	// Filter tags by prefix if specified
	var filteredTags []*v1pb.TagWithMemos
	for _, tag := range tagMap {
		if request.PathPrefix == "" || strings.HasPrefix(tag.TagNode.Name, request.PathPrefix) {
			// Include memo IDs only if requested
			if !request.IncludeMemoIds {
				tag.TagNode.MemoIds = nil
			}
			filteredTags = append(filteredTags, tag)
		}
	}

	// Sort tags by name for consistent ordering
	sort.Slice(filteredTags, func(i, j int) bool {
		return filteredTags[i].TagNode.Name < filteredTags[j].TagNode.Name
	})

	// Add hierarchy information if requested (default: true)
	includeHierarchy := request.IncludeHierarchy
	if includeHierarchy {
		s.addHierarchyInformation(filteredTags)
	}

	return &v1pb.ListTagsResponse{
		Tags:       filteredTags,
		TotalCount: int32(len(filteredTags)),
	}, nil
}

// GetTag gets specific tag details
func (s *APIV1Service) GetTag(ctx context.Context, request *v1pb.GetTagRequest) (*v1pb.GetTagResponse, error) {
	user, err := s.GetCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user")
	}

	// URL decode the tag path
	tagPath, err := url.PathUnescape(request.TagPath)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tag path: %v", err)
	}

	// Get all memos for the current user
	memos, err := s.Store.ListMemos(ctx, &store.FindMemo{
		CreatorID:       &user.ID,
		ExcludeComments: true,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list memos: %v", err)
	}

	// Aggregate tags from all memos
	tagMap, err := s.aggregateTagsFromMemos(memos, user.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to aggregate tags: %v", err)
	}

	// Find the requested tag
	tag, exists := tagMap[tagPath]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "tag not found: %s", tagPath)
	}

	// Include memo IDs by default for GetTag
	if !request.IncludeMemoIds {
		tag.TagNode.MemoIds = nil
	}

	// Add hierarchy information
	allTags := make([]*v1pb.TagWithMemos, 0, len(tagMap))
	for _, t := range tagMap {
		allTags = append(allTags, t)
	}
	s.addHierarchyInformation(allTags)

	// Find the tag again after hierarchy processing
	for _, t := range allTags {
		if t.TagNode.Name == tagPath {
			return &v1pb.GetTagResponse{Tag: t}, nil
		}
	}

	return &v1pb.GetTagResponse{Tag: tag}, nil
}

// RenameTag renames a tag globally (supports path moving)
func (s *APIV1Service) RenameTag(ctx context.Context, request *v1pb.RenameTagRequest) (*v1pb.RenameTagResponse, error) {
	user, err := s.GetCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user")
	}

	// URL decode the tag paths
	oldTagPath, err := url.PathUnescape(request.OldTagPath)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid old tag path: %v", err)
	}

	newTagPath := request.NewTagPath
	if newTagPath == "" {
		return nil, status.Errorf(codes.InvalidArgument, "new tag path cannot be empty")
	}

	// Validate new tag path format (should start with / for hierarchical tags)
	if !strings.HasPrefix(newTagPath, "/") {
		newTagPath = "/" + newTagPath
	}

	// Get all memos that contain the old tag
	var tagFilter string
	if request.MoveChildren {
		// Find all tags that start with the old path
		tagFilter = fmt.Sprintf("tag in [\"%s\"]", oldTagPath)
	} else {
		// Only exact match
		tagFilter = fmt.Sprintf("tag in [\"%s\"]", oldTagPath)
	}

	memos, err := s.Store.ListMemos(ctx, &store.FindMemo{
		CreatorID:       &user.ID,
		Filters:         []string{tagFilter},
		ExcludeComments: true,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list memos with tag: %v", err)
	}

	if len(memos) == 0 {
		return nil, status.Errorf(codes.NotFound, "no memos found with tag: %s", oldTagPath)
	}

	affectedMemoIDs := make([]string, 0, len(memos))
	renamedPaths := make(map[string]string)

	// Update each memo's content and payload
	for _, memo := range memos {
		updated := false
		newContent := memo.Content
		
		// Update content by replacing tag references
		if request.MoveChildren {
			// Replace all occurrences of tags that start with oldTagPath
			oldPrefix := "#" + strings.TrimPrefix(oldTagPath, "/")
			newPrefix := "#" + strings.TrimPrefix(newTagPath, "/")
			
			// Simple string replacement for now
			// TODO: Use proper markdown parser for more accurate replacement
			newContent = strings.ReplaceAll(newContent, oldPrefix, newPrefix)
			updated = true
		} else {
			// Only replace exact tag matches
			oldTag := "#" + strings.TrimPrefix(oldTagPath, "/")
			newTag := "#" + strings.TrimPrefix(newTagPath, "/")
			newContent = strings.ReplaceAll(newContent, oldTag, newTag)
			updated = true
		}

		if updated {
			// Update memo content
			err := s.Store.UpdateMemo(ctx, &store.UpdateMemo{
				ID:      memo.ID,
				Content: &newContent,
			})
			if err != nil {
				return nil, status.Errorf(codes.Internal, "failed to update memo content: %v", err)
			}

			affectedMemoIDs = append(affectedMemoIDs, memo.UID)
			renamedPaths[oldTagPath] = newTagPath
		}
	}

	return &v1pb.RenameTagResponse{
		AffectedMemoIds: affectedMemoIDs,
		RenamedPaths:    renamedPaths,
	}, nil
}

// DeleteTag deletes a tag from all content
func (s *APIV1Service) DeleteTag(ctx context.Context, request *v1pb.DeleteTagRequest) (*v1pb.DeleteTagResponse, error) {
	user, err := s.GetCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user")
	}

	// URL decode the tag path
	tagPath, err := url.PathUnescape(request.TagPath)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tag path: %v", err)
	}

	// Get all memos that contain this tag
	tagFilter := fmt.Sprintf("tag in [\"%s\"]", tagPath)
	memos, err := s.Store.ListMemos(ctx, &store.FindMemo{
		CreatorID:       &user.ID,
		Filters:         []string{tagFilter},
		ExcludeComments: true,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list memos with tag: %v", err)
	}

	affectedMemoIDs := make([]string, 0, len(memos))
	deletedTagPaths := []string{tagPath}

	switch request.Strategy {
	case v1pb.DeleteTagRequest_REMOVE_FROM_CONTENT:
		// Remove tag from memo content
		for _, memo := range memos {
			newContent := memo.Content
			tagToRemove := "#" + strings.TrimPrefix(tagPath, "/")
			
			// Remove the tag from content
			// TODO: Use proper markdown parser for more accurate removal
			newContent = strings.ReplaceAll(newContent, tagToRemove+" ", "")
			newContent = strings.ReplaceAll(newContent, tagToRemove, "")
			
			// Clean up extra spaces
			newContent = strings.TrimSpace(newContent)

			// Update memo content
			err := s.Store.UpdateMemo(ctx, &store.UpdateMemo{
				ID:      memo.ID,
				Content: &newContent,
			})
			if err != nil {
				return nil, status.Errorf(codes.Internal, "failed to update memo content: %v", err)
			}

			affectedMemoIDs = append(affectedMemoIDs, memo.UID)
		}

	case v1pb.DeleteTagRequest_DELETE_RELATED_MEMOS:
		// Delete all memos that contain this tag
		for _, memo := range memos {
			err := s.Store.DeleteMemo(ctx, &store.DeleteMemo{ID: memo.ID})
			if err != nil {
				return nil, status.Errorf(codes.Internal, "failed to delete memo: %v", err)
			}
			affectedMemoIDs = append(affectedMemoIDs, memo.UID)
		}

	default:
		return nil, status.Errorf(codes.InvalidArgument, "unsupported delete strategy")
	}

	return &v1pb.DeleteTagResponse{
		AffectedMemoIds:   affectedMemoIDs,
		DeletedTagPaths:   deletedTagPaths,
	}, nil
}

// aggregateTagsFromMemos extracts and aggregates tags from a list of memos
func (s *APIV1Service) aggregateTagsFromMemos(memos []*store.Memo, creatorID int32) (map[string]*v1pb.TagWithMemos, error) {
	tagMap := make(map[string]*v1pb.TagWithMemos)

	for _, memo := range memos {
		if memo.Payload == nil || len(memo.Payload.Tags) == 0 {
			continue
		}

		for _, tag := range memo.Payload.Tags {
			tagPath := tag.Name
			if tagPath == "" {
				continue
			}

			// Ensure tag path starts with /
			if !strings.HasPrefix(tagPath, "/") {
				tagPath = "/" + tagPath
			}

			// Create or update TagWithMemos
			if existing, exists := tagMap[tagPath]; exists {
				// Add memo ID if not already present
				if !contains(existing.TagNode.MemoIds, memo.UID) {
					existing.TagNode.MemoIds = append(existing.TagNode.MemoIds, memo.UID)
				}
				existing.DirectMemoCount++
			} else {
				// Create new tag entry
				pathSegments := strings.Split(strings.Trim(tagPath, "/"), "/")
				if len(pathSegments) == 1 && pathSegments[0] == "" {
					pathSegments = []string{}
				}

				tagMap[tagPath] = &v1pb.TagWithMemos{
					TagNode: &storepb.TagNode{
						Name:         tagPath,
						PathSegments: pathSegments,
						MemoIds:      []string{memo.UID},
						CreatorId:    creatorID,
					},
					DirectMemoCount: 1,
					TotalMemoCount:  1, // Will be calculated later with hierarchy
				}
			}
		}
	}

	return tagMap, nil
}

// addHierarchyInformation adds parent/child relationships to tags
func (s *APIV1Service) addHierarchyInformation(tags []*v1pb.TagWithMemos) {
	tagMap := make(map[string]*v1pb.TagWithMemos)
	for _, tag := range tags {
		tagMap[tag.TagNode.Name] = tag
	}

	for _, tag := range tags {
		tagPath := tag.TagNode.Name
		
		// Find children
		for otherPath := range tagMap {
			if otherPath != tagPath && strings.HasPrefix(otherPath, tagPath+"/") {
				// Check if it's a direct child (not grandchild)
				remainder := strings.TrimPrefix(otherPath, tagPath+"/")
				if !strings.Contains(remainder, "/") {
					tag.ChildPaths = append(tag.ChildPaths, otherPath)
				}
			}
		}

		// Find parent
		if lastSlash := strings.LastIndex(tagPath, "/"); lastSlash > 0 {
			parentPath := tagPath[:lastSlash]
			if _, exists := tagMap[parentPath]; exists {
				tag.ParentPath = parentPath
			}
		}

		// Calculate total memo count (including children)
		tag.TotalMemoCount = s.calculateTotalMemoCount(tag, tagMap, make(map[string]bool))
	}
}

// calculateTotalMemoCount recursively calculates total memo count including children
func (s *APIV1Service) calculateTotalMemoCount(tag *v1pb.TagWithMemos, tagMap map[string]*v1pb.TagWithMemos, visited map[string]bool) int32 {
	if visited[tag.TagNode.Name] {
		return 0 // Avoid infinite loops
	}
	visited[tag.TagNode.Name] = true

	total := tag.DirectMemoCount
	for _, childPath := range tag.ChildPaths {
		if child, exists := tagMap[childPath]; exists {
			total += s.calculateTotalMemoCount(child, tagMap, visited)
		}
	}

	return total
}

// contains checks if a string slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}