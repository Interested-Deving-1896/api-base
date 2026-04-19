// Logger middleware emits one structured log line per request with method,
// path, status, duration, and client IP. The request ID is automatically
// attached via logger.FromContext.
//
// This is the single most useful line of observability in the entire app.
// When something goes wrong, the first thing you do is find the request in
// these logs.
package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/topboyasante/api-base/internal/observability/logger"
)

func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.FromContext(c.Request.Context()).Info("http_request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
		)
	}
}
