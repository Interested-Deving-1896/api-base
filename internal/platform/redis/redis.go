// Package redis provides a configured Redis client.
//
// We use Redis for rate limiting counters and (in the future) caching.
// The client is safe to share across goroutines. Call New once at startup
// and pass the result to anything that needs Redis.
package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/topboyasante/api-base/internal/config"
)

func New(cfg config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
	})
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return client, nil
}
