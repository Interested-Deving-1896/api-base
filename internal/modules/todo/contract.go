// Package todo exposes the public contract for the todo module — the
// interface that other modules import when they need to talk to todos.
//
// If another module (say, a notification module) wants to look up a todo,
// it takes a todo.TodoService in its constructor. It never imports
// internal/modules/todo/service directly. This keeps module boundaries
// clear and makes dependencies explicit.
//
// For now, no other module depends on todos, but defining the contract
// from day one sets the habit.
package todo

import "github.com/topboyasante/api-base/internal/modules/todo/service"


type TodoService = service.Service