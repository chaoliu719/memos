package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/usememos/memos/store"
)

func (d *DB) CreateCategory(ctx context.Context, create *store.Category) (*store.Category, error) {
	fields := []string{"name", "path", "parent_id", "creator_id", "color", "icon"}
	placeholder := []string{"$1", "$2", "$3", "$4", "$5", "$6"}
	args := []any{create.Name, create.Path, create.ParentID, create.CreatorID, create.Color, create.Icon}

	stmt := "INSERT INTO category (" + strings.Join(fields, ", ") + ") VALUES (" + strings.Join(placeholder, ", ") + ") RETURNING id, created_ts, updated_ts, row_status"
	if err := d.db.QueryRowContext(ctx, stmt, args...).Scan(
		&create.ID,
		&create.CreatedTs,
		&create.UpdatedTs,
		&create.RowStatus,
	); err != nil {
		return nil, err
	}

	return create, nil
}

func (d *DB) ListCategories(ctx context.Context, find *store.FindCategory) ([]*store.Category, error) {
	where, args := []string{"1 = 1"}, []any{}
	argIndex := 1

	if v := find.ID; v != nil {
		where, args = append(where, fmt.Sprintf("id = $%d", argIndex)), append(args, *v)
		argIndex++
	}
	if v := find.CreatorID; v != nil {
		where, args = append(where, fmt.Sprintf("creator_id = $%d", argIndex)), append(args, *v)
		argIndex++
	}
	if v := find.RowStatus; v != nil {
		where, args = append(where, fmt.Sprintf("row_status = $%d", argIndex)), append(args, *v)
		argIndex++
	}
	if v := find.Name; v != nil {
		where, args = append(where, fmt.Sprintf("name = $%d", argIndex)), append(args, *v)
		argIndex++
	}
	if v := find.Path; v != nil {
		where, args = append(where, fmt.Sprintf("path = $%d", argIndex)), append(args, *v)
		argIndex++
	}
	if v := find.ParentID; v != nil {
		where, args = append(where, fmt.Sprintf("parent_id = $%d", argIndex)), append(args, *v)
		argIndex++
	}

	orderBy := []string{}
	if find.OrderByName {
		orderBy = append(orderBy, "name ASC")
	}
	if find.OrderByPath {
		orderBy = append(orderBy, "path ASC")
	}
	if len(orderBy) == 0 {
		orderBy = append(orderBy, "created_ts DESC")
	}

	fields := []string{
		"id",
		"name",
		"path",
		"parent_id",
		"creator_id",
		"color",
		"icon",
		"created_ts",
		"updated_ts",
		"row_status",
	}

	query := fmt.Sprintf("SELECT %s FROM category WHERE %s ORDER BY %s", strings.Join(fields, ", "), strings.Join(where, " AND "), strings.Join(orderBy, ", "))

	// Add pagination
	if find.Limit != nil {
		query += fmt.Sprintf(" LIMIT %d", *find.Limit)
		if find.Offset != nil {
			query += fmt.Sprintf(" OFFSET %d", *find.Offset)
		}
	}

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []*store.Category
	for rows.Next() {
		category := &store.Category{}
		var parentID sql.NullInt32

		if err := rows.Scan(
			&category.ID,
			&category.Name,
			&category.Path,
			&parentID,
			&category.CreatorID,
			&category.Color,
			&category.Icon,
			&category.CreatedTs,
			&category.UpdatedTs,
			&category.RowStatus,
		); err != nil {
			return nil, err
		}

		if parentID.Valid {
			category.ParentID = &parentID.Int32
		}

		categories = append(categories, category)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return categories, nil
}

func (d *DB) UpdateCategory(ctx context.Context, update *store.UpdateCategory) error {
	set, args := []string{}, []any{}
	argIndex := 1

	if v := update.UpdatedTs; v != nil {
		set, args = append(set, fmt.Sprintf("updated_ts = $%d", argIndex)), append(args, *v)
		argIndex++
	}
	if v := update.RowStatus; v != nil {
		set, args = append(set, fmt.Sprintf("row_status = $%d", argIndex)), append(args, *v)
		argIndex++
	}
	if v := update.Name; v != nil {
		set, args = append(set, fmt.Sprintf("name = $%d", argIndex)), append(args, *v)
		argIndex++
	}
	if v := update.Path; v != nil {
		set, args = append(set, fmt.Sprintf("path = $%d", argIndex)), append(args, *v)
		argIndex++
	}
	if v := update.ParentID; v != nil {
		set, args = append(set, fmt.Sprintf("parent_id = $%d", argIndex)), append(args, *v)
		argIndex++
	}
	if v := update.Color; v != nil {
		set, args = append(set, fmt.Sprintf("color = $%d", argIndex)), append(args, *v)
		argIndex++
	}
	if v := update.Icon; v != nil {
		set, args = append(set, fmt.Sprintf("icon = $%d", argIndex)), append(args, *v)
		argIndex++
	}

	args = append(args, update.ID)

	stmt := fmt.Sprintf("UPDATE category SET %s WHERE id = $%d", strings.Join(set, ", "), argIndex)
	if _, err := d.db.ExecContext(ctx, stmt, args...); err != nil {
		return err
	}

	return nil
}

func (d *DB) DeleteCategory(ctx context.Context, delete *store.DeleteCategory) error {
	stmt := "DELETE FROM category WHERE id = $1"
	if _, err := d.db.ExecContext(ctx, stmt, delete.ID); err != nil {
		return err
	}

	return nil
}