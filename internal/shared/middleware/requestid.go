// Package middleware contains HTTP middleware that runs for every request.
// Each file in this package is one piece of middleware; wire.go decides the
// order they run in.
//
// RequestID must always be first. It generates (or accepts via header) a
// unique ID for the request, stores it in context, and echoes it in the
// X-Request-ID response header. Every other middleware and every handler
// relies on the ID being present.
package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/topboyasante/api-base/internal/shared/requestctx"
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}
		ctx := requestctx.WithRequestID(c.Request.Context(), id)
		c.Request = c.Request.WithContext(ctx)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}
