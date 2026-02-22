package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/sentinel-app/sentinel/internal/apperrors"
	"github.com/sentinel-app/sentinel/internal/models"
	"gorm.io/gorm"
)

// ContactService handles saved contact CRUD operations.
type ContactService struct {
	db *gorm.DB
}

// NewContactService creates a new ContactService.
func NewContactService(db *gorm.DB) *ContactService {
	return &ContactService{db: db}
}

// SavedContactInput holds the data for creating or updating a saved contact.
type SavedContactInput struct {
	Email string `json:"email" binding:"required,email"`
	Name  string `json:"name"`
}

// ListContacts returns all saved contacts for a user.
func (s *ContactService) ListContacts(ctx context.Context, userID uuid.UUID) ([]models.Connection, error) {
	var connections []models.Connection
	result := s.db.Where("entity_id = ?", userID).
		Order("created_at ASC").
		Find(&connections)
	if result.Error != nil {
		return nil, result.Error
	}
	return connections, nil
}

// CreateContact adds a new saved contact for a user.
func (s *ContactService) CreateContact(ctx context.Context, userID uuid.UUID, input SavedContactInput) (*models.Connection, error) {
	// Check for duplicate email for this user
	var existing models.Connection
	result := s.db.Where("entity_id = ? AND signal = ?", userID, input.Email).First(&existing)
	if result.Error == nil {
		return nil, apperrors.ErrContactDuplicate
	}

	connection := models.Connection{
		EntityID: userID,
		Signal:   input.Email,
		Alias:    input.Name,
	}

	if err := s.db.Create(&connection).Error; err != nil {
		return nil, fmt.Errorf("failed to create contact: %w", err)
	}

	return &connection, nil
}

// UpdateContact updates a saved contact's name and/or email.
func (s *ContactService) UpdateContact(ctx context.Context, contactID, userID uuid.UUID, input SavedContactInput) (*models.Connection, error) {
	var connection models.Connection
	result := s.db.Where("id = ? AND entity_id = ?", contactID, userID).First(&connection)
	if result.Error != nil {
		return nil, apperrors.ErrContactNotFound
	}

	// If email is changing, check for duplicate
	if input.Email != connection.Signal {
		var existing models.Connection
		dup := s.db.Where("entity_id = ? AND signal = ? AND id != ?", userID, input.Email, contactID).First(&existing)
		if dup.Error == nil {
			return nil, apperrors.ErrContactDuplicate
		}
	}

	connection.Signal = input.Email
	connection.Alias = input.Name

	if err := s.db.Save(&connection).Error; err != nil {
		return nil, fmt.Errorf("failed to update contact: %w", err)
	}

	return &connection, nil
}

// DeleteContact removes a saved contact.
func (s *ContactService) DeleteContact(ctx context.Context, contactID, userID uuid.UUID) error {
	result := s.db.Where("id = ? AND entity_id = ?", contactID, userID).Delete(&models.Connection{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete contact: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrContactNotFound
	}
	return nil
}
