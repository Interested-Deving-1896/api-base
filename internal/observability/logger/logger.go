// Package logger wraps Go's standard slog library with a helper that
// automatically attaches request-scoped values (like request_id) to every
// log line.
//
// Call Init once at startup (main.go does this). After that, anywhere in
// the codebase that has a context.Context can call:
//
//	logger.FromContext(ctx).Info("user_created", "user_id", u.ID)
//
// and the resulting log line will include request_id automatically. This
// is how we trace a request across many functions without passing a
// logger down every call.
//
// In production we emit JSON (parseable by log aggregators). In dev we
// emit human-readable text. The format is chosen by the APP_ENV value.
package logger

import (
	"context"
	"log/slog"
	"os"

	"github.com/topboyasante/api-base/internal/shared/requestctx"
)

var base *slog.Logger

func Init(env string) {
	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if env != "production" {
		opts.Level = slog.LevelDebug
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	base = slog.New(handler)
	slog.SetDefault(base)
}

// FromContext returns a logger with request_id (and any other request-scoped
// fields) pre-attached. Always prefer this over calling slog directly, so
// that every log line is automatically correlated to a request.
func FromContext(ctx context.Context) *slog.Logger {
	if base == nil {
		// Init wasn't called. Fall back to default so tests don't panic.
		return slog.Default()
	}
	l := base
	if rid := requestctx.RequestID(ctx); rid != "" {
		l = l.With("request_id", rid)
	}
	return l
}
