// Package domain holds the internal business model for the todo module.
// This struct represents a Todo as our code thinks about it, independent
// of how it's stored in the database or sent over the API.
//
// Domain models never leave this module. They are not sent as API
// responses (that's what DTOs are for). They are not imported by other
// modules (those modules use the todo.TodoService contract instead).
package domain

import "time"

type Todo struct {
	ID          string
	Title       string
	Description string
	Done        bool
	CreatedAt   time.Time
}