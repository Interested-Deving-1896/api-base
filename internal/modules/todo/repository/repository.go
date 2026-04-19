// Package repository is the data-access layer for the todo module. It
// translates domain.Todo values to/from rows in the todos table.
//
// The Repository interface is what the service depends on. Tests can
// provide a mock implementation; production uses the real sqlx-backed one.
//
// Error translation happens here: sql.ErrNoRows becomes apierror.ErrNotFound,
// and unique violations become apierror.ErrConflict. The service and
// handler layers don't need to know about SQL details.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/topboyasante/api-base/internal/modules/todo/domain"
	"github.com/topboyasante/api-base/internal/shared/apierror"
)

type Repository interface {
	Create(ctx context.Context, t *domain.Todo) error
	GetByID(ctx context.Context, id string) (*domain.Todo, error)
}

type repo struct {
	db *sqlx.DB
}

func New(db *sqlx.DB) Repository {
	return &repo{db: db}
}

type todoRow struct {
	ID          string    `db:"id"`
	Title       string    `db:"title"`
	Description string    `db:"description"`
	Done        bool      `db:"done"`
	CreatedAt   time.Time `db:"created_at"`
}

func (r *repo) Create(ctx context.Context, t *domain.Todo) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO todos (id, title, description, done, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, t.ID, t.Title, t.Description, t.Done, t.CreatedAt)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return apierror.ErrConflict
		}
		return err
	}
	return nil
}

func (r *repo) GetByID(ctx context.Context, id string) (*domain.Todo, error) {
	var row todoRow
	err := r.db.GetContext(ctx, &row, `
		SELECT id, title, description, done, created_at
		FROM todos WHERE id = $1
	`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apierror.ErrNotFound
		}
		return nil, err
	}
	return &domain.Todo{
		ID:          row.ID,
		Title:       row.Title,
		Description: row.Description,
		Done:        row.Done,
		CreatedAt:   row.CreatedAt,
	}, nil
}
