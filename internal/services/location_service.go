package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/sentinel-app/sentinel/internal/apperrors"
	"github.com/sentinel-app/sentinel/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LocationService handles GPS coordinate updates.
type LocationService struct {
	db *gorm.DB
}

// NewLocationService creates a new LocationService.
func NewLocationService(db *gorm.DB) *LocationService {
	return &LocationService{db: db}
}

// UpdateLocationInput holds GPS data from the client.
type UpdateLocationInput struct {
	Lat      float64 `json:"lat" binding:"required"`
	Lng      float64 `json:"lng" binding:"required"`
	Accuracy float64 `json:"accuracy"`
}

// UpdateLocation upserts the latest GPS coordinates for a user.
func (s *LocationService) UpdateLocation(ctx context.Context, userID uuid.UUID, input UpdateLocationInput) error {
	coord := models.Coordinate{
		EntityID: userID,
		Lat:      input.Lat,
		Lng:      input.Lng,
		Accuracy: input.Accuracy,
	}

	// Upsert: update if exists, insert if not
	result := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "entity_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"lat", "lng", "accuracy", "updated_at"}),
	}).Create(&coord)

	if result.Error != nil {
		return fmt.Errorf("failed to update location: %w", result.Error)
	}
	return nil
}

// GetLocation retrieves the latest location for a user.
func (s *LocationService) GetLocation(ctx context.Context, userID uuid.UUID) (*models.Coordinate, error) {
	var coord models.Coordinate
	result := s.db.Where("entity_id = ?", userID).First(&coord)
	if result.Error != nil {
		return nil, apperrors.ErrLocationNotFound
	}
	return &coord, nil
}
