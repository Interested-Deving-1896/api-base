// Package response defines the shape every API response takes — success,
// error, or validation failure. Every handler writes its response through
// the helpers here (Success, Error, ValidationError). This is how we
// guarantee a consistent response format across the whole API.
//
// Handlers should never call c.JSON directly. If you find yourself wanting
// to, ask why — the helpers probably already handle your case, or we should
// add a new helper rather than diverge.
//
// The request_id field inside every error body is populated from context.
// That ID comes from the RequestID middleware, which sets it before any
// handler runs. If you see an error response without a request_id, the
// middleware isn't wired up correctly.
package response

import (
	"github.com/gin-gonic/gin"
	"github.com/topboyasante/api-base/internal/shared/requestctx"
)

type Envelope struct {
	Success bool       `json:"success"`
	Data    any        `json:"data,omitempty"`
	Error   *ErrorBody `json:"error,omitempty"`
	Meta    *Meta      `json:"meta,omitempty"`
}

type ErrorBody struct {
	Code      string       `json:"code"`
	Message   string       `json:"message"`
	RequestID string       `json:"request_id"`
	Details   []FieldError `json:"details,omitempty"`
}

type FieldError struct {
	Field   string `json:"field"`
	Tag     string `json:"tag"`
	Message string `json:"message"`
}

type Meta struct {
	Page       int `json:"page,omitempty"`
	PageSize   int `json:"page_size,omitempty"`
	TotalItems int `json:"total_items,omitempty"`
}

func Success(c *gin.Context, status int, data any) {
	c.JSON(status, Envelope{Success: true, Data: data})
}

func Error(c *gin.Context, status int, code, message string) {
	c.JSON(status, Envelope{
		Success: false,
		Error: &ErrorBody{
			Code:      code,
			Message:   message,
			RequestID: requestctx.RequestID(c.Request.Context()),
		},
	})
}

func ValidationError(c *gin.Context, fields []FieldError) {
	c.JSON(422, Envelope{
		Success: false,
		Error: &ErrorBody{
			Code:      "VALIDATION_FAILED",
			Message:   "the request contains invalid data",
			RequestID: requestctx.RequestID(c.Request.Context()),
			Details:   fields,
		},
	})
}