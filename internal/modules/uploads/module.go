package uploads

import (
	"github.com/topboyasante/api-base/internal/modules/uploads/handler"
	"github.com/topboyasante/api-base/internal/modules/uploads/service"
	"github.com/topboyasante/api-base/internal/providers/storage"
)

// New wires the uploads module's internals and returns its HTTP handler.
// Called from internal/app/wire.go during application startup.
//
// The module takes a storage.Provider rather than a specific backend so
// the same code works whether files go to local disk, S3, or R2 — that
// choice is made in wire.go based on config.
func New(s storage.Provider) *handler.Handler {
	svc := service.NewService(s)
	return handler.NewHandler(svc)
}
