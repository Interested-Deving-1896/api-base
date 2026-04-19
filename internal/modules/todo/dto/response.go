package dto

import "time"

// TodoResponse is what we send back to clients. Notice this has JSON tags
// but no validation tags. It also has no pointer to anything internal —
// handlers build it via the mapper package.
type TodoResponse struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Done        bool      `json:"done"`
	CreatedAt   time.Time `json:"created_at"`
}
