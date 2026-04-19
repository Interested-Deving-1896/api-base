package todo

import (
	govalidator "github.com/go-playground/validator/v10"
	"github.com/jmoiron/sqlx"

	"github.com/topboyasante/api-base/internal/modules/todo/handler"
	"github.com/topboyasante/api-base/internal/modules/todo/repository"
	"github.com/topboyasante/api-base/internal/modules/todo/service"
)

// New wires the todo module's internals and returns its HTTP handler.
// Called from internal/app/wire.go during application startup.
func New(db *sqlx.DB, v *govalidator.Validate) *handler.Handler {
	repo := repository.New(db)
	svc := service.New(repo)
	return handler.New(svc, v)
}