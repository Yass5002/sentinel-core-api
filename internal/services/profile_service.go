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
	db           *gorm.DB
	eventService *EventService
	fileService  *FileService
}

// NewProfileService creates a new ProfileService.
func NewProfileService(db *gorm.DB, eventService *EventService, fileService *FileService) *ProfileService {
	return &ProfileService{db: db, eventService: eventService, fileService: fileService}
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

// DeleteAccount permanently deletes a user, their S3 files, active DB records, timers, and the account itself.
func (s *ProfileService) DeleteAccount(ctx context.Context, userID uuid.UUID) error {
	// 1. Terminate active events and clear Redis timers
	if err := s.eventService.ClearUserTimers(ctx, userID); err != nil {
		return fmt.Errorf("failed to clear user timers: %w", err)
	}

	// 2. Clear all user files from S3
	if err := s.fileService.DeleteAllUserFilesFromS3(ctx, userID); err != nil {
		return fmt.Errorf("failed to clear user files from S3: %w", err)
	}

	// 3. Delete all related DB records transactionally using Unscoped to ensure permanent deletion
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("sequence_id IN (SELECT id FROM sequences WHERE entity_id = ?)", userID).Delete(&models.Endpoint{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("entity_id = ?", userID).Delete(&models.Artifact{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("entity_id = ?", userID).Delete(&models.Sequence{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("entity_id = ?", userID).Delete(&models.Connection{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("entity_id = ?", userID).Delete(&models.Coordinate{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("id = ?", userID).Delete(&models.Entity{}).Error; err != nil {
			return err
		}
		return nil
	})
}
