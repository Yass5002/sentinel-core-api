package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sentinel-app/sentinel/internal/apperrors"
	"github.com/sentinel-app/sentinel/internal/middleware"
	"github.com/sentinel-app/sentinel/internal/services"
)

const (
	minDurationMS = 30_000               // 30 seconds
	maxDurationMS = 7 * 24 * 3600 * 1000 // 7 days in ms
	maxContacts   = 10
	maxNameLen    = 100
	maxMessageLen = 2000
)

// EventHandler handles event CRUD endpoints.
type EventHandler struct {
	eventService *services.EventService
}

// NewEventHandler creates a new EventHandler.
func NewEventHandler(eventService *services.EventService) *EventHandler {
	return &EventHandler{eventService: eventService}
}

type createEventRequest struct {
	Name       string                  `json:"name" binding:"required"`
	Message    string                  `json:"message"`
	DurationMS int64                   `json:"duration_ms" binding:"required,gt=0"`
	Contacts   []services.ContactInput `json:"contacts" binding:"required,min=1,dive"`
	FileIDs    []string                `json:"file_ids"`
}

// CreateEvent handles POST /events
func (h *EventHandler) CreateEvent(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		return
	}

	var req createEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	// Trim whitespace
	req.Name = strings.TrimSpace(req.Name)
	req.Message = strings.TrimSpace(req.Message)

	// Validate name is not whitespace-only
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name cannot be empty or whitespace"})
		return
	}
	if len(req.Name) > maxNameLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name too long (max 100 characters)"})
		return
	}

	// Validate message length
	if len(req.Message) > maxMessageLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message too long (max 2000 characters)"})
		return
	}

	// Validate duration bounds
	if req.DurationMS < minDurationMS {
		c.JSON(http.StatusBadRequest, gin.H{"error": "duration_ms must be at least 30000 (30 seconds)"})
		return
	}
	if req.DurationMS > maxDurationMS {
		c.JSON(http.StatusBadRequest, gin.H{"error": "duration_ms cannot exceed 604800000 (7 days)"})
		return
	}

	// Validate contacts count
	if len(req.Contacts) > maxContacts {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many contacts (max 10)"})
		return
	}

	// Deduplicate and trim contact info
	seen := make(map[string]bool)
	var deduped []services.ContactInput
	for _, contact := range req.Contacts {
		email := strings.TrimSpace(strings.ToLower(contact.Email))
		name := strings.TrimSpace(contact.Name)
		if seen[email] {
			continue // skip duplicate
		}
		seen[email] = true
		deduped = append(deduped, services.ContactInput{
			Email: email,
			Name:  name,
		})
	}

	if len(deduped) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one contact with a valid email is required"})
		return
	}

	// Parse optional file IDs
	var fileIDs []uuid.UUID
	for _, raw := range req.FileIDs {
		id, err := parseUUID(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file_id: " + raw})
			return
		}
		fileIDs = append(fileIDs, id)
	}

	input := services.CreateEventInput{
		UserID:     userID,
		Name:       req.Name,
		Message:    req.Message,
		DurationMS: req.DurationMS,
		Contacts:   deduped,
		FileIDs:    fileIDs,
	}

	event, err := h.eventService.CreateEvent(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, apperrors.ErrFileAttachment) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, event)
}

// GetEvent handles GET /events/:id
func (h *EventHandler) GetEvent(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		return
	}

	eventID, err := parseUUID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event ID"})
		return
	}

	event, err := h.eventService.GetEvent(c.Request.Context(), eventID, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, event)
}

// ListEvents handles GET /events
func (h *EventHandler) ListEvents(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		return
	}

	filter := c.DefaultQuery("filter", "all")

	var events []interface{}
	var err error

	switch filter {
	case "active":
		result, e := h.eventService.ListActiveEvents(c.Request.Context(), userID)
		err = e
		for _, ev := range result {
			events = append(events, ev)
		}
	default:
		result, e := h.eventService.ListEvents(c.Request.Context(), userID)
		err = e
		for _, ev := range result {
			events = append(events, ev)
		}
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"events": events})
}

// CheckIn handles POST /events/:id/checkin
func (h *EventHandler) CheckIn(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		return
	}

	eventID, err := parseUUID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event ID"})
		return
	}

	if err := h.eventService.CheckIn(c.Request.Context(), eventID, userID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "checked in — event terminated"})
}

// CancelEvent handles DELETE /events/:id
func (h *EventHandler) CancelEvent(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		return
	}

	eventID, err := parseUUID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event ID"})
		return
	}

	if err := h.eventService.CancelEvent(c.Request.Context(), eventID, userID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "event cancelled"})
}
