package v1

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1pb "github.com/usememos/memos/proto/gen/api/v1"
	"github.com/usememos/memos/store"
)

const (
	CategoryNamePrefix = "categories/"
)

func (s *APIV1Service) CreateCategory(ctx context.Context, request *v1pb.CreateCategoryRequest) (*v1pb.Category, error) {
	user, err := s.GetCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user")
	}

	create := &store.Category{
		CreatorID: user.ID,
		Name:      request.Category.DisplayName,
		Color:     request.Category.Color,
		Icon:      request.Category.Icon,
	}

	// Handle parent category
	if request.Category.Parent != "" {
		parentID, err := extractCategoryID(request.Category.Parent)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid parent category name: %v", err)
		}

		// Verify parent exists and belongs to the same user
		normalStatus := store.Normal
		parent, err := s.Store.GetCategory(ctx, &store.FindCategory{
			ID:        &parentID,
			CreatorID: &user.ID,
			RowStatus: &normalStatus,
		})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get parent category: %v", err)
		}
		if parent == nil {
			return nil, status.Errorf(codes.NotFound, "parent category not found")
		}

		create.ParentID = &parentID
	}

	category, err := s.Store.CreateCategory(ctx, create)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create category: %v", err)
	}

	return convertCategoryFromStore(category), nil
}

func (s *APIV1Service) ListCategories(ctx context.Context, request *v1pb.ListCategoriesRequest) (*v1pb.ListCategoriesResponse, error) {
	user, err := s.GetCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user")
	}

	normalStatus := store.Normal
	find := &store.FindCategory{
		CreatorID:   &user.ID,
		RowStatus:   &normalStatus,
		OrderByPath: true,
	}

	// Apply pagination
	limit := DefaultPageSize
	if request.PageSize > 0 && request.PageSize <= MaxPageSize {
		limit = int(request.PageSize)
	}
	find.Limit = &limit

	offset := 0
	if request.PageToken != "" {
		pageToken := &v1pb.PageToken{}
		if err := unmarshalPageToken(request.PageToken, pageToken); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid page token")
		}
		offset = int(pageToken.Offset)
	}
	find.Offset = &offset

	categories, err := s.Store.ListCategories(ctx, find)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list categories: %v", err)
	}

	var nextPageToken string
	if len(categories) == limit {
		nextPageToken, _ = getPageToken(limit, offset+limit)
	}

	response := &v1pb.ListCategoriesResponse{
		Categories:    []*v1pb.Category{},
		NextPageToken: nextPageToken,
	}

	for _, category := range categories {
		response.Categories = append(response.Categories, convertCategoryFromStore(category))
	}

	return response, nil
}

func (s *APIV1Service) GetCategory(ctx context.Context, request *v1pb.GetCategoryRequest) (*v1pb.Category, error) {
	user, err := s.GetCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user")
	}

	categoryID, err := extractCategoryID(request.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid category name: %v", err)
	}

	normalStatus := store.Normal
	category, err := s.Store.GetCategory(ctx, &store.FindCategory{
		ID:        &categoryID,
		CreatorID: &user.ID,
		RowStatus: &normalStatus,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get category: %v", err)
	}
	if category == nil {
		return nil, status.Errorf(codes.NotFound, "category not found")
	}

	return convertCategoryFromStore(category), nil
}

func (s *APIV1Service) UpdateCategory(ctx context.Context, request *v1pb.UpdateCategoryRequest) (*v1pb.Category, error) {
	user, err := s.GetCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user")
	}

	categoryID, err := extractCategoryID(request.Category.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid category name: %v", err)
	}

	// Verify category exists and belongs to the user
	normalStatus := store.Normal
	existing, err := s.Store.GetCategory(ctx, &store.FindCategory{
		ID:        &categoryID,
		CreatorID: &user.ID,
		RowStatus: &normalStatus,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get category: %v", err)
	}
	if existing == nil {
		return nil, status.Errorf(codes.NotFound, "category not found")
	}

	update := &store.UpdateCategory{
		ID: categoryID,
	}

	if request.UpdateMask == nil {
		request.UpdateMask = &fieldmaskpb.FieldMask{
			Paths: []string{"display_name", "parent", "color", "icon", "state"},
		}
	}

	// Apply updates based on field mask
	for _, field := range request.UpdateMask.Paths {
		switch field {
		case "display_name":
			update.Name = &request.Category.DisplayName
		case "parent":
			if request.Category.Parent != "" {
				parentID, err := extractCategoryID(request.Category.Parent)
				if err != nil {
					return nil, status.Errorf(codes.InvalidArgument, "invalid parent category name: %v", err)
				}

				// Verify parent exists and belongs to the same user
				normalStatus := store.Normal
				parent, err := s.Store.GetCategory(ctx, &store.FindCategory{
					ID:        &parentID,
					CreatorID: &user.ID,
					RowStatus: &normalStatus,
				})
				if err != nil {
					return nil, status.Errorf(codes.Internal, "failed to get parent category: %v", err)
				}
				if parent == nil {
					return nil, status.Errorf(codes.NotFound, "parent category not found")
				}

				update.ParentID = &parentID
			} else {
				// Clear parent (make it a root category)
				update.ParentID = nil
			}
		case "color":
			update.Color = &request.Category.Color
		case "icon":
			update.Icon = &request.Category.Icon
		case "state":
			rowStatus := convertStateToStore(request.Category.State)
			update.RowStatus = &rowStatus
		}
	}

	if err := s.Store.UpdateCategory(ctx, update); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update category: %v", err)
	}

	// Get the updated category
	updated, err := s.Store.GetCategory(ctx, &store.FindCategory{
		ID:        &categoryID,
		CreatorID: &user.ID,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get updated category: %v", err)
	}

	return convertCategoryFromStore(updated), nil
}

func (s *APIV1Service) DeleteCategory(ctx context.Context, request *v1pb.DeleteCategoryRequest) (*emptypb.Empty, error) {
	user, err := s.GetCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user")
	}

	categoryID, err := extractCategoryID(request.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid category name: %v", err)
	}

	// Verify category exists and belongs to the user
	normalStatus := store.Normal
	existing, err := s.Store.GetCategory(ctx, &store.FindCategory{
		ID:        &categoryID,
		CreatorID: &user.ID,
		RowStatus: &normalStatus,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get category: %v", err)
	}
	if existing == nil {
		return nil, status.Errorf(codes.NotFound, "category not found")
	}

	// Check if there are any child categories
	normalStatus2 := store.Normal
	children, err := s.Store.ListCategories(ctx, &store.FindCategory{
		ParentID:  &categoryID,
		CreatorID: &user.ID,
		RowStatus: &normalStatus2,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to check child categories: %v", err)
	}
	if len(children) > 0 {
		return nil, status.Errorf(codes.FailedPrecondition, "cannot delete category with child categories")
	}

	// Check if there are any memos using this category
	// TODO: Implement this check once memo-category integration is complete

	if err := s.Store.DeleteCategory(ctx, &store.DeleteCategory{ID: categoryID}); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete category: %v", err)
	}

	return &emptypb.Empty{}, nil
}

func (s *APIV1Service) GetCategoryHierarchy(ctx context.Context, request *v1pb.GetCategoryHierarchyRequest) (*v1pb.GetCategoryHierarchyResponse, error) {
	user, err := s.GetCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user")
	}

	categories, err := s.Store.GetCategoryHierarchy(ctx, user.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get category hierarchy: %v", err)
	}

	response := &v1pb.GetCategoryHierarchyResponse{
		Categories: []*v1pb.Category{},
	}

	for _, category := range categories {
		response.Categories = append(response.Categories, convertCategoryFromStore(category))
	}

	return response, nil
}

// Helper functions

func extractCategoryID(name string) (int32, error) {
	if !strings.HasPrefix(name, CategoryNamePrefix) {
		return 0, fmt.Errorf("invalid category name format")
	}

	idStr := strings.TrimPrefix(name, CategoryNamePrefix)
	id, err := strconv.ParseInt(idStr, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid category ID")
	}

	return int32(id), nil
}

func convertCategoryFromStore(category *store.Category) *v1pb.Category {
	pb := &v1pb.Category{
		Name:        fmt.Sprintf("%s%d", CategoryNamePrefix, category.ID),
		Id:          category.ID,
		DisplayName: category.Name,
		Path:        category.Path,
		Color:       category.Color,
		Icon:        category.Icon,
		CreateTime:  timestamppb.New(time.Unix(category.CreatedTs, 0)),
		UpdateTime:  timestamppb.New(time.Unix(category.UpdatedTs, 0)),
		State:       convertStateFromStore(category.RowStatus),
	}

	if category.ParentID != nil {
		pb.Parent = fmt.Sprintf("%s%d", CategoryNamePrefix, *category.ParentID)
	}

	return pb
}