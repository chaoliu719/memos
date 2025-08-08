package store

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// Category represents the category model.
type Category struct {
	// ID is the system generated unique identifier for the category.
	ID int32

	// Standard fields
	RowStatus RowStatus
	CreatorID int32
	CreatedTs int64
	UpdatedTs int64

	// Domain specific fields
	Name     string
	Path     string
	ParentID *int32
	Color    string
	Icon     string
}

type FindCategory struct {
	ID *int32

	// Standard fields
	RowStatus *RowStatus
	CreatorID *int32

	// Domain specific fields
	Name     *string
	Path     *string
	ParentID *int32

	// Pagination
	Limit  *int
	Offset *int

	// Ordering
	OrderByName bool
	OrderByPath bool
}

type UpdateCategory struct {
	ID        int32
	UpdatedTs *int64
	RowStatus *RowStatus
	Name      *string
	Path      *string
	ParentID  *int32
	Color     *string
	Icon      *string
}

type DeleteCategory struct {
	ID int32
}

// Category validation constants
const (
	MaxCategoryNameLength = 100
	MaxCategoryPathLength = 500
	MaxCategoryDepth      = 10
	MaxCategoryIconLength = 20
)

var (
	// CategoryNameRegex validates category name format
	CategoryNameRegex = regexp.MustCompile(`^[\p{L}\p{N}\s\-_\.]{1,100}$`)
	// CategoryColorRegex validates hex color format
	CategoryColorRegex = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
)

func (s *Store) CreateCategory(ctx context.Context, create *Category) (*Category, error) {
	// Validate category name
	if create.Name == "" || len(create.Name) > MaxCategoryNameLength {
		return nil, errors.New("invalid category name length")
	}
	if !CategoryNameRegex.MatchString(create.Name) {
		return nil, errors.New("invalid category name format")
	}

	// Validate color format
	if create.Color != "" && !CategoryColorRegex.MatchString(create.Color) {
		return nil, errors.New("invalid color format")
	}

	// Validate icon length
	if len(create.Icon) > MaxCategoryIconLength {
		return nil, errors.New("invalid icon length")
	}

	// Set defaults
	if create.Color == "" {
		create.Color = "#6366f1"
	}
	if create.Icon == "" {
		create.Icon = "📁"
	}

	// Build and validate path
	if err := s.buildCategoryPath(ctx, create); err != nil {
		return nil, err
	}

	// Validate path constraints
	if err := s.validateCategoryPath(ctx, create.Path, create.CreatorID); err != nil {
		return nil, err
	}

	return s.driver.CreateCategory(ctx, create)
}

func (s *Store) ListCategories(ctx context.Context, find *FindCategory) ([]*Category, error) {
	return s.driver.ListCategories(ctx, find)
}

func (s *Store) GetCategory(ctx context.Context, find *FindCategory) (*Category, error) {
	list, err := s.ListCategories(ctx, find)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}

	category := list[0]
	return category, nil
}

func (s *Store) UpdateCategory(ctx context.Context, update *UpdateCategory) error {
	// Get existing category
	existing, err := s.GetCategory(ctx, &FindCategory{ID: &update.ID})
	if err != nil {
		return err
	}
	if existing == nil {
		return errors.New("category not found")
	}

	// Validate name if being updated
	if update.Name != nil {
		if *update.Name == "" || len(*update.Name) > MaxCategoryNameLength {
			return errors.New("invalid category name length")
		}
		if !CategoryNameRegex.MatchString(*update.Name) {
			return errors.New("invalid category name format")
		}
	}

	// Validate color if being updated
	if update.Color != nil && *update.Color != "" && !CategoryColorRegex.MatchString(*update.Color) {
		return errors.New("invalid color format")
	}

	// Validate icon if being updated
	if update.Icon != nil && len(*update.Icon) > MaxCategoryIconLength {
		return errors.New("invalid icon length")
	}

	// If name or parent is being updated, rebuild path
	if update.Name != nil || update.ParentID != nil {
		newCategory := *existing
		if update.Name != nil {
			newCategory.Name = *update.Name
		}
		if update.ParentID != nil {
			newCategory.ParentID = update.ParentID
		}

		if err := s.buildCategoryPath(ctx, &newCategory); err != nil {
			return err
		}

		// Validate new path
		if err := s.validateCategoryPath(ctx, newCategory.Path, existing.CreatorID); err != nil {
			return err
		}

		update.Path = &newCategory.Path

		// Update child categories' paths if name changed
		if update.Name != nil && *update.Name != existing.Name {
			if err := s.updateChildCategoryPaths(ctx, existing); err != nil {
				return err
			}
		}
	}

	return s.driver.UpdateCategory(ctx, update)
}

func (s *Store) DeleteCategory(ctx context.Context, delete *DeleteCategory) error {
	return s.driver.DeleteCategory(ctx, delete)
}

// GetCategoryHierarchy returns the full hierarchy for a user's categories
func (s *Store) GetCategoryHierarchy(ctx context.Context, creatorID int32) ([]*Category, error) {
	normalStatus := Normal
	return s.ListCategories(ctx, &FindCategory{
		CreatorID:   &creatorID,
		RowStatus:   &normalStatus,
		OrderByPath: true,
	})
}

// buildCategoryPath constructs the full path for a category
func (s *Store) buildCategoryPath(ctx context.Context, category *Category) error {
	if category.ParentID == nil {
		category.Path = category.Name
		return nil
	}

	// Get parent category
	normalStatus := Normal
	parent, err := s.GetCategory(ctx, &FindCategory{
		ID:        category.ParentID,
		CreatorID: &category.CreatorID,
		RowStatus: &normalStatus,
	})
	if err != nil {
		return err
	}
	if parent == nil {
		return errors.New("parent category not found")
	}

	// Build path
	category.Path = path.Join(parent.Path, category.Name)
	return nil
}

// validateCategoryPath validates path constraints
func (s *Store) validateCategoryPath(ctx context.Context, categoryPath string, creatorID int32) error {
	// Check path length
	if len(categoryPath) > MaxCategoryPathLength {
		return errors.New("category path too long")
	}

	// Check depth
	depth := len(strings.Split(categoryPath, "/"))
	if depth > MaxCategoryDepth {
		return fmt.Errorf("category depth exceeds maximum of %d levels", MaxCategoryDepth)
	}

	// Check for duplicate paths
	normalStatus := Normal
	existing, err := s.GetCategory(ctx, &FindCategory{
		Path:      &categoryPath,
		CreatorID: &creatorID,
		RowStatus: &normalStatus,
	})
	if err != nil {
		return err
	}
	if existing != nil {
		return errors.New("category path already exists")
	}

	return nil
}

// updateChildCategoryPaths updates all child category paths when parent name changes
func (s *Store) updateChildCategoryPaths(ctx context.Context, parentCategory *Category) error {
	// Find all child categories
	normalStatus := Normal
	children, err := s.ListCategories(ctx, &FindCategory{
		ParentID:  &parentCategory.ID,
		CreatorID: &parentCategory.CreatorID,
		RowStatus: &normalStatus,
	})
	if err != nil {
		return err
	}

	// Update each child's path recursively
	for _, child := range children {
		if err := s.buildCategoryPath(ctx, child); err != nil {
			return err
		}

		// Update the child category
		if err := s.driver.UpdateCategory(ctx, &UpdateCategory{
			ID:   child.ID,
			Path: &child.Path,
		}); err != nil {
			return err
		}

		// Recursively update grandchildren
		if err := s.updateChildCategoryPaths(ctx, child); err != nil {
			return err
		}
	}

	return nil
}