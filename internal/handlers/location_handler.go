package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sentinel-app/sentinel/internal/middleware"
	"github.com/sentinel-app/sentinel/internal/services"
)

// LocationHandler handles GPS location endpoints.
type LocationHandler struct {
	locationService *services.LocationService
}

// NewLocationHandler creates a new LocationHandler.
func NewLocationHandler(locationService *services.LocationService) *LocationHandler {
	return &LocationHandler{locationService: locationService}
}

// UpdateLocation handles POST /location
func (h *LocationHandler) UpdateLocation(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		return
	}

	var req services.UpdateLocationInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	if err := h.locationService.UpdateLocation(c.Request.Context(), userID, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "location updated"})
}

// GetLocation handles GET /location
func (h *LocationHandler) GetLocation(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		return
	}

	location, err := h.locationService.GetLocation(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, location)
}
