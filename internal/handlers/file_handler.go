package handlers

import (
	"fmt"
	"net/http"

	"github.com/gabriel-vasile/mimetype"
	"github.com/gin-gonic/gin"
	"github.com/sentinel-app/sentinel/internal/config"
	"github.com/sentinel-app/sentinel/internal/middleware"
	"github.com/sentinel-app/sentinel/internal/services"
)

// FileHandler handles staged file upload endpoints.
type FileHandler struct {
	fileService *services.FileService
	cfg         *config.Config
}

// NewFileHandler creates a new FileHandler.
func NewFileHandler(fileService *services.FileService, cfg *config.Config) *FileHandler {
	return &FileHandler{fileService: fileService, cfg: cfg}
}

// Upload handles POST /files — stage a file for later event attachment.
func (h *FileHandler) Upload(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no file provided"})
		return
	}
	defer file.Close()

	// Check file size
	maxBytes := int64(h.cfg.MaxFileSizeMB) * 1024 * 1024
	if header.Size > maxBytes {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("file too large, max %dMB", h.cfg.MaxFileSizeMB),
		})
		return
	}

	// Detect MIME type from content rather than trusting the client header.
	// We read the first 3072 bytes for detection, then the full upload streams from the beginning.
	mtype, err := mimetype.DetectReader(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unable to detect file type"})
		return
	}
	detectedMimeType := mtype.String()

	// Seek back to start so S3 upload reads the full file
	if _, err := file.Seek(0, 0); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process file"})
		return
	}

	artifact, err := h.fileService.UploadStaged(
		c.Request.Context(),
		userID,
		header.Filename,
		header.Size,
		detectedMimeType,
		file,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, artifact)
}

// ListStaged handles GET /files — list staged (unattached) files.
func (h *FileHandler) ListStaged(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		return
	}

	files, err := h.fileService.ListStagedFiles(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, files)
}

// DeleteStaged handles DELETE /files/:id — delete a staged file.
func (h *FileHandler) DeleteStaged(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		return
	}

	fileID, err := parseUUID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file ID"})
		return
	}

	if err := h.fileService.DeleteStagedFile(c.Request.Context(), fileID, userID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "file deleted"})
}
