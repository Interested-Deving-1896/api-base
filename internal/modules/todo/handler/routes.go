package handler

import "github.com/gin-gonic/gin"

// RegisterQueryRoutes wires the read-only endpoints. These don't need
// idempotency middleware; GETs are already idempotent.
func (h *Handler) RegisterQueryRoutes(r *gin.RouterGroup) {
	g := r.Group("/todos")
	g.GET("/:id", h.GetByID)
}

// RegisterMutationRoutes wires the write endpoints. wire.go wraps this
// group in idempotency middleware.
func (h *Handler) RegisterMutationRoutes(r *gin.RouterGroup) {
	g := r.Group("/todos")
	g.POST("", h.Create)
}
