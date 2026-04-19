// Package server wraps *http.Server with a graceful shutdown helper.
//
// When the process receives SIGINT or SIGTERM, we want to:
//  1. Stop accepting new connections
//  2. Let in-flight requests complete (up to a timeout)
//  3. Close the server cleanly
//
// This matters in production because the orchestrator (Kubernetes,
// systemd) will SIGTERM our pod during deploys. Without graceful
// shutdown, in-flight requests get dropped and users see random errors.
//
// Usage: build a server with New, then call Run. Run blocks until the
// process receives a shutdown signal or the server errors.
package server

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Server struct {
	httpServer *http.Server
}

func New(addr string, handler http.Handler) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
		},
	}
}

func (s *Server) Run() error {
	// Start listening in a goroutine so Run can also wait for signals.
	errCh := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// Wait for either a fatal server error or a shutdown signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case <-sigCh:
		// Got shutdown signal; drain in-flight requests.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(ctx)
	}
}
