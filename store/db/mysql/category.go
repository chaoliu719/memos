package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/usememos/memos/store"
)

func (d *DB) CreateCategory(ctx context.Context, create *store.Category) (*store.Category, error) {
	fields := []string{"`name`", "`path`", "`parent_id`", "`creator_id`", "`color`", "`icon`"}
	placeholder := []string{"?", "?", "?", "?", "?", "?"}
	args := []any{create.Name, create.Path, create.ParentID, create.CreatorID, create.Color, create.Icon}

	stmt := "INSERT INTO `category` (" + strings.Join(fields, ", ") + ") VALUES (" + strings.Join(placeholder, ", ") + ")"
	result, err := d.db.ExecContext(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}

	// Get the auto-generated ID
	rawID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	create.ID = int32(rawID)

	// Fetch the complete record to get timestamps and row_status
	row := d.db.QueryRowContext(ctx, "SELECT `created_ts`, `updated_ts`, `row_status` FROM `category` WHERE `id` = ?", create.ID)
	if err := row.Scan(&create.CreatedTs, &create.UpdatedTs, &create.RowStatus); err != nil {
		return nil, err
	}

	return create, nil
}

func (d *DB) ListCategories(ctx context.Context, find *store.FindCategory) ([]*store.Category, error) {
	where, args := []string{"1 = 1"}, []any{}

	if v := find.ID; v != nil {
		where, args = append(where, "`id` = ?"), append(args, *v)
	}
	if v := find.CreatorID; v != nil {
		where, args = append(where, "`creator_id` = ?"), append(args, *v)
	}
	if v := find.RowStatus; v != nil {
		where, args = append(where, "`row_status` = ?"), append(args, *v)
	}
	if v := find.Name; v != nil {
		where, args = append(where, "`name` = ?"), append(args, *v)
	}
	if v := find.Path; v != nil {
		where, args = append(where, "`path` = ?"), append(args, *v)
	}
	if v := find.ParentID; v != nil {
		where, args = append(where, "`parent_id` = ?"), append(args, *v)
	}

	orderBy := []string{}
	if find.OrderByName {
		orderBy = append(orderBy, "`name` ASC")
	}
	if find.OrderByPath {
		orderBy = append(orderBy, "`path` ASC")
	}
	if len(orderBy) == 0 {
		orderBy = append(orderBy, "`created_ts` DESC")
	}

	fields := []string{
		"`id`",
		"`name`",
		"`path`",
		"`parent_id`",
		"`creator_id`",
		"`color`",
		"`icon`",
		"`created_ts`",
		"`updated_ts`",
		"`row_status`",
	}

	query := fmt.Sprintf("SELECT %s FROM `category` WHERE %s ORDER BY %s", strings.Join(fields, ", "), strings.Join(where, " AND "), strings.Join(orderBy, ", "))

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

	if v := update.UpdatedTs; v != nil {
		set, args = append(set, "`updated_ts` = ?"), append(args, *v)
	}
	if v := update.RowStatus; v != nil {
		set, args = append(set, "`row_status` = ?"), append(args, *v)
	}
	if v := update.Name; v != nil {
		set, args = append(set, "`name` = ?"), append(args, *v)
	}
	if v := update.Path; v != nil {
		set, args = append(set, "`path` = ?"), append(args, *v)
	}
	if v := update.ParentID; v != nil {
		set, args = append(set, "`parent_id` = ?"), append(args, *v)
	}
	if v := update.Color; v != nil {
		set, args = append(set, "`color` = ?"), append(args, *v)
	}
	if v := update.Icon; v != nil {
		set, args = append(set, "`icon` = ?"), append(args, *v)
	}

	args = append(args, update.ID)

	stmt := fmt.Sprintf("UPDATE `category` SET %s WHERE `id` = ?", strings.Join(set, ", "))
	if _, err := d.db.ExecContext(ctx, stmt, args...); err != nil {
		return err
	}

	return nil
}

func (d *DB) DeleteCategory(ctx context.Context, delete *store.DeleteCategory) error {
	stmt := "DELETE FROM `category` WHERE `id` = ?"
	if _, err := d.db.ExecContext(ctx, stmt, delete.ID); err != nil {
		return err
	}

	return nil
}