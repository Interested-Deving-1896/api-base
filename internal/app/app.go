// Package app assembles the application from its components and runs it.
//
// BuildApp (in wire.go) is called from main.go with a loaded Config. It
// connects to infrastructure (db, redis), builds shared utilities, wires
// every module, and sets up the HTTP router with middleware.
//
// The App struct holds the assembled pieces so main.go can call Run.
package app

import (
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
	"github.com/topboyasante/api-base/internal/platform/server"
	"github.com/topboyasante/api-base/internal/providers/storage"
)

type App struct {
	router *gin.Engine
	db     *sqlx.DB
	redis  *redis.Client
	port   string

	storage storage.Provider
}

func (a *App) Run() error {
	srv := server.New(":"+a.port, a.router)
	return srv.Run()
}
