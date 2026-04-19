// Recover middleware catches panics that escape handlers and turns them
// into 500 responses. Without this, a panic would crash the whole process.
//
// We log the panic (with stack trace and request ID) so you can debug it,
// and return a generic error to the client so we don't leak internals.
package middleware

import (
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/topboyasante/api-base/internal/observability/logger"
	"github.com/topboyasante/api-base/internal/shared/response"
)

func Recover() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.FromContext(c.Request.Context()).Error("panic_recovered",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				response.Error(c, 500, "INTERNAL", "an internal error occurred")
				c.Abort()
			}
		}()
		c.Next()
	}
}
