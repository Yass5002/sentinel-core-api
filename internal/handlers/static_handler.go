package handlers

import (
	"github.com/gin-gonic/gin"
)

// StaticHandler handles static HTML pages.
type StaticHandler struct{}

// NewStaticHandler creates a new StaticHandler.
func NewStaticHandler() *StaticHandler {
	return &StaticHandler{}
}

// GetPrivacy serves the privacy policy page.
func (h *StaticHandler) GetPrivacy(c *gin.Context) {
	c.File("web/static/privacy.html")
}

// GetTerms serves the terms of service page.
func (h *StaticHandler) GetTerms(c *gin.Context) {
	c.File("web/static/terms.html")
}
