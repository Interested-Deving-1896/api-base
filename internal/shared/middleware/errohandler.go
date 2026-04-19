// ErrorHandler middleware runs after all handlers and converts any errors
// they added to c.Errors into properly formatted HTTP responses.
//
// The contract is:
//   - Handlers call c.Error(err) and return, rather than calling
//     response.Error directly.
//   - If the error is an *apierror.Error, we use its HTTPCode, Code, and
//     user-safe Message.
//   - Otherwise, we log the real error (never expose it) and return a
//     generic 500 with the request ID.
//
// This is how we enforce "never leak internals in errors" across the
// whole codebase. A handler can't accidentally return a Postgres error
// string to the user — the middleware intercepts it.
package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/topboyasante/api-base/internal/observability/logger"
	"github.com/topboyasante/api-base/internal/shared/apierror"
	"github.com/topboyasante/api-base/internal/shared/response"
)

func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) == 0 {
			return
		}

		err := c.Errors.Last().Err
		logger.FromContext(c.Request.Context()).Error("handler_error", "err", err.Error())

		if apiErr, ok := apierror.As(err); ok {
			response.Error(c, apiErr.HTTPCode, apiErr.Code, apiErr.Message)
			return
		}
		response.Error(c, 500, "INTERNAL", "an internal error occurred")
	}
}
