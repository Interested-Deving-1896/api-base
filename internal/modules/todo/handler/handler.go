// Package handler is the HTTP-facing layer of the todo module. It does
// four things per endpoint:
//
//  1. Bind and validate the incoming DTO
//  2. Call the service with primitive values
//  3. Map the returned domain model to a response DTO
//  4. Send the response through the response package
//
// Handlers are deliberately thin. Business logic belongs in the service.
// Data access belongs in the repository. If you find yourself writing a
// SQL query or a business rule in a handler, move it down a layer.
//
// Swaggo annotations (the `// @Summary`, `// @Success` lines) document
// each endpoint. Running `make docs` regenerates api/docs from these
// annotations. CI fails the build if they drift.
package handler

import (
	"github.com/gin-gonic/gin"
	govalidator "github.com/go-playground/validator/v10"

	"github.com/topboyasante/api-base/internal/modules/todo/dto"
	"github.com/topboyasante/api-base/internal/modules/todo/mapper"
	"github.com/topboyasante/api-base/internal/modules/todo/service"
	platformvalidator "github.com/topboyasante/api-base/internal/platform/validator"
	"github.com/topboyasante/api-base/internal/shared/response"
)

// SuccessResponse is a swaggo helper — it documents the envelope shape
// returned by successful endpoints. Use it in @Success annotations:
//
//	@Success 201 {object} handler.SuccessResponse{data=dto.TodoResponse}
type SuccessResponse struct {
	Success bool `json:"success" example:"true"`
	Data    any  `json:"data"`
}

// ErrorResponse documents the envelope shape returned by error endpoints.
type ErrorResponse struct {
	Success bool               `json:"success" example:"false"`
	Error   response.ErrorBody `json:"error"`
}

type Handler struct {
	svc       service.Service
	validator *govalidator.Validate
}

func New(svc service.Service, v *govalidator.Validate) *Handler {
	return &Handler{svc: svc, validator: v}
}

// Create godoc
// @Summary      Create a todo
// @Tags         todos
// @Accept       json
// @Produce      json
// @Param        body body dto.CreateTodoRequest true "todo payload"
// @Success      201 {object} handler.SuccessResponse{data=dto.TodoResponse}
// @Failure      422 {object} handler.ErrorResponse
// @Router       /todos [post]
func (h *Handler) Create(c *gin.Context) {
	var req dto.CreateTodoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fields := platformvalidator.TranslateErrors(err)
		if len(fields) > 0 {
			response.ValidationError(c, fields)
			return
		}
		response.Error(c, 400, "BAD_REQUEST", "invalid request body")
		return
	}

	t, err := h.svc.Create(c.Request.Context(), req.Title, req.Description)
	if err != nil {
		c.Error(err)
		return
	}

	response.Success(c, 201, mapper.ToTodoResponse(t))
}

// GetByID godoc
// @Summary      Get a todo by ID
// @Tags         todos
// @Produce      json
// @Param        id path string true "todo ID"
// @Success      200 {object} handler.SuccessResponse{data=dto.TodoResponse}
// @Failure      404 {object} handler.ErrorResponse
// @Router       /todos/{id} [get]
func (h *Handler) GetByID(c *gin.Context) {
	id := c.Param("id")
	t, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}
	response.Success(c, 200, mapper.ToTodoResponse(t))
}
