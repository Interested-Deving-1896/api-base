// Package ratelimit provides Redis-backed rate limiting for our HTTP API.
//
// Design choices worth knowing:
//
//   - We use an hourly fixed-window counter. Simpler than a sliding window,
//     fine for most APIs. The key is "ratelimit:{bucket}:{YYYY-MM-DD-HH}",
//     so each hour gets its own counter and old counters expire naturally.
//
//   - When Redis is unreachable, we FAIL OPEN — log a warning and let the
//     request through. The rate limiter is a safety net, not a security
//     boundary. (Contrast with idempotency, which fails closed.)
//
//   - We set X-RateLimit-* headers on every response so clients can back
//     off gracefully before they hit the limit.
//
// The Middleware function buckets requests by client IP. If you need
// per-user or per-API-key limits later, add a bucketKey parameter and
// compose another middleware.
package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/topboyasante/api-base/internal/observability/logger"
	"github.com/topboyasante/api-base/internal/shared/response"
)

type Limiter struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Limiter {
	return &Limiter{rdb: rdb}
}

type Result struct {
	Allowed   bool
	Remaining int
	Limit     int
	ResetAt   time.Time
}

func (l *Limiter) Check(ctx context.Context, bucketKey string, limit int) (*Result, error) {
	hour := time.Now().UTC().Format("2006-01-02-15")
	key := fmt.Sprintf("ratelimit:%s:%s", bucketKey, hour)

	count, err := l.rdb.Incr(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if count == 1 {
		l.rdb.Expire(ctx, key, time.Hour)
	}

	remaining := limit - int(count)
	if remaining < 0 {
		remaining = 0
	}
	resetAt := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)

	return &Result{
		Allowed:   int(count) <= limit,
		Remaining: remaining,
		Limit:     limit,
		ResetAt:   resetAt,
	}, nil
}

func Middleware(l *Limiter, perIPLimit int) gin.HandlerFunc {
	return func(c *gin.Context) {
		res, err := l.Check(c.Request.Context(), "ip:"+c.ClientIP(), perIPLimit)
		if err != nil {
			logger.FromContext(c.Request.Context()).Warn("ratelimit_check_failed", "err", err)
			c.Next()
			return
		}

		c.Header("X-RateLimit-Limit", strconv.Itoa(res.Limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(res.ResetAt.Unix(), 10))

		if !res.Allowed {
			c.Header("Retry-After", strconv.Itoa(int(time.Until(res.ResetAt).Seconds())))
			response.Error(c, 429, "RATE_LIMIT_EXCEEDED",
				fmt.Sprintf("rate limit exceeded, resets at %s", res.ResetAt.Format(time.RFC3339)))
			c.Abort()
			return
		}
		c.Next()
	}
}
