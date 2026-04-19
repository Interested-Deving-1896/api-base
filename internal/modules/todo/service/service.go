// Package service holds the business logic for the todo module. Handlers
// call into the Service interface; the service calls into the Repository.
//
// The service is where domain rules live: default values, cross-field
// validation, coordination between the database and other subsystems. For
// now, "create a todo" is straightforward — we set an ID and timestamp,
// then save. As the business grows, this is where those rules will land.
package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/topboyasante/api-base/internal/modules/todo/domain"
	"github.com/topboyasante/api-base/internal/modules/todo/repository"
	"github.com/topboyasante/api-base/internal/observability/logger"
)

type Service interface {
	Create(ctx context.Context, title, description string) (*domain.Todo, error)
	GetByID(ctx context.Context, id string) (*domain.Todo, error)
}

type service struct {
	repo repository.Repository
}

func New(r repository.Repository) Service {
	return &service{repo: r}
}

func (s *service) Create(ctx context.Context, title, description string) (*domain.Todo, error) {
	t := &domain.Todo{
		ID:          uuid.NewString(),
		Title:       title,
		Description: description,
		Done:        false,
		CreatedAt:   time.Now().UTC(),
	}
	if err := s.repo.Create(ctx, t); err != nil {
		logger.FromContext(ctx).Error("todo_create_failed", "err", err.Error())
		return nil, err
	}
	logger.FromContext(ctx).Info("todo_created", "todo_id", t.ID)
	return t, nil
}

func (s *service) GetByID(ctx context.Context, id string) (*domain.Todo, error) {
	return s.repo.GetByID(ctx, id)
}