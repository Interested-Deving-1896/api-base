// Package main is the entry point for the API binary.
//
// This file should stay short. Its only jobs are:
//  1. Load config
//  2. Build the application (see internal/app/wire.go)
//  3. Run it
//
// If you're tempted to put anything else here — a handler, a database
// query, a helper function — put it in the appropriate package instead.
// Keeping main.go minimal means anyone can read it and immediately know
// where to look for real code.
//
// The top-level swaggo annotations (@title, @version, @BasePath) are read
// by `swag init` to populate the generated OpenAPI spec.
package main

import (
	"log"

	_ "github.com/topboyasante/api-base/api/docs"
	"github.com/topboyasante/api-base/internal/app"
	"github.com/topboyasante/api-base/internal/config"
)

// @title           Backend API
// @version         1.0
// @description     Modular monolith backend for learning
// @BasePath        /api/v1
func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	a, err := app.Build(cfg)
	if err != nil {
		log.Fatalf("build app: %v", err)
	}

	if err := a.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
