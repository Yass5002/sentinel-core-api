package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sentinel-app/sentinel/internal/apperrors"
	"github.com/sentinel-app/sentinel/internal/middleware"
	"github.com/sentinel-app/sentinel/internal/services"
)

// ContactHandler handles saved contact CRUD endpoints.
type ContactHandler struct {
	contactService *services.ContactService
}

// NewContactHandler creates a new ContactHandler.
func NewContactHandler(contactService *services.ContactService) *ContactHandler {
	return &ContactHandler{contactService: contactService}
}

type createContactRequest struct {
	Email string `json:"email" binding:"required,email"`
	Name  string `json:"name"`
}

type updateContactRequest struct {
	Email string `json:"email" binding:"required,email"`
	Name  string `json:"name"`
}

// ListContacts handles GET /contacts
func (h *ContactHandler) ListContacts(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		return
	}

	contacts, err := h.contactService.ListContacts(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"contacts": contacts})
}

// CreateContact handles POST /contacts
func (h *ContactHandler) CreateContact(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		return
	}

	var req createContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	contact, err := h.contactService.CreateContact(c.Request.Context(), userID, services.SavedContactInput{
		Email: req.Email,
		Name:  req.Name,
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrContactDuplicate) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, contact)
}

// UpdateContact handles PATCH /contacts/:id
func (h *ContactHandler) UpdateContact(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		return
	}

	contactID, err := parseUUID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid contact ID"})
		return
	}

	var req updateContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	contact, err := h.contactService.UpdateContact(c.Request.Context(), contactID, userID, services.SavedContactInput{
		Email: req.Email,
		Name:  req.Name,
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrContactNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, apperrors.ErrContactDuplicate) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, contact)
}

// DeleteContact handles DELETE /contacts/:id
func (h *ContactHandler) DeleteContact(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		return
	}

	contactID, err := parseUUID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid contact ID"})
		return
	}

	if err := h.contactService.DeleteContact(c.Request.Context(), contactID, userID); err != nil {
		if errors.Is(err, apperrors.ErrContactNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "contact deleted"})
}
