package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	uploadsModule "github.com/topboyasante/api-base/internal/modules/uploads/service"
	"github.com/topboyasante/api-base/internal/shared/response"
)

// SuccessResponse documents the envelope shape for successful uploads —
// used by swaggo in @Success annotations.
type SuccessResponse struct {
	Success bool `json:"success" example:"true"`
	Data    any  `json:"data"`
}

// ErrorResponse documents the envelope shape for failed uploads.
type ErrorResponse struct {
	Success bool               `json:"success" example:"false"`
	Error   response.ErrorBody `json:"error"`
}

type Handler struct {
	svc *uploadsModule.Service
}

func NewHandler(svc *uploadsModule.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	r.POST("/upload", h.Upload)
}

// Upload godoc
// @Summary      Upload a file
// @Description  Accepts a multipart file and stores it via the configured storage provider (local, s3, or r2). Max size 10MB.
// @Tags         uploads
// @Accept       multipart/form-data
// @Produce      json
// @Param        file formData file true "file to upload"
// @Success      201 {object} handler.SuccessResponse{data=storage.UploadResult}
// @Failure      400 {object} handler.ErrorResponse
// @Failure      413 {object} handler.ErrorResponse
// @Failure      500 {object} handler.ErrorResponse
// @Router       /upload [post]
func (h *Handler) Upload(c *gin.Context) {
	// 10MB limit — adjust for your use case. Keep this reasonable; huge
	// uploads should use multipart streaming or direct-to-S3 presigned URLs.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10<<20)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.Error(c, 400, "BAD_REQUEST", "missing or invalid 'file' field")
		return
	}
	defer file.Close()

	result, err := h.svc.Upload(
		c.Request.Context(),
		header.Filename,
		file,
		header.Size,
		header.Header.Get("Content-Type"),
	)
	if err != nil {
		response.Error(c, 500, "UPLOAD_FAILED", "upload failed")
		return
	}

	response.Success(c, 201, result)
}