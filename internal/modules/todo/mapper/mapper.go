// Package mapper converts between domain models and DTOs.
//
// Two separate files would be overkill for now, but as the module grows
// you might split this into todo_to_dto.go and todo_from_dto.go. For now,
// one function is enough.
//
// IMPORTANT: the mapper is the ONLY place where domain <-> DTO conversion
// happens. Handlers don't construct DTOs directly from domain fields; they
// call the mapper. This way, if the domain model changes, only the mapper
// needs updating.
package mapper

import (
	"github.com/topboyasante/api-base/internal/modules/todo/domain"
	"github.com/topboyasante/api-base/internal/modules/todo/dto"
)

func ToTodoResponse(t *domain.Todo) dto.TodoResponse {
	return dto.TodoResponse{
		ID:          t.ID,
		Title:       t.Title,
		Description: t.Description,
		Done:        t.Done,
		CreatedAt:   t.CreatedAt,
	}
}
