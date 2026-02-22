package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/sentinel-app/sentinel/internal/apperrors"
	"github.com/sentinel-app/sentinel/internal/models"
	"gorm.io/gorm"
)

// ProfileService handles user profile operations.
type ProfileService struct {
	db *gorm.DB
}

// NewProfileService creates a new ProfileService.
func NewProfileService(db *gorm.DB) *ProfileService {
	return &ProfileService{db: db}
}

// GetProfile retrieves a user's profile.
func (s *ProfileService) GetProfile(ctx context.Context, userID uuid.UUID) (*models.Entity, error) {
	var entity models.Entity
	result := s.db.First(&entity, "id = ?", userID)
	if result.Error != nil {
		return nil, apperrors.ErrUserNotFound
	}
	return &entity, nil
}

// UpdateProfile updates a user's display name.
func (s *ProfileService) UpdateProfile(ctx context.Context, userID uuid.UUID, name string) error {
	result := s.db.Model(&models.Entity{}).Where("id = ?", userID).Update("alias", name)
	if result.Error != nil {
		return fmt.Errorf("failed to update profile: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrUserNotFound
	}
	return nil
}
