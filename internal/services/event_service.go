package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sentinel-app/sentinel/internal/apperrors"
	"github.com/sentinel-app/sentinel/internal/models"
	"gorm.io/gorm"
)

// EventService handles event CRUD and check-in logic.
type EventService struct {
	db          *gorm.DB
	rdb         *redis.Client
	fileService *FileService
}

// NewEventService creates a new EventService.
func NewEventService(db *gorm.DB, rdb *redis.Client, fileService *FileService) *EventService {
	return &EventService{db: db, rdb: rdb, fileService: fileService}
}

// CreateEventInput holds the data needed to create a new event.
type CreateEventInput struct {
	UserID     uuid.UUID
	Name       string
	Message    string
	DurationMS int64 // timer duration in milliseconds
	Contacts   []ContactInput
	FileIDs    []uuid.UUID // optional staged file IDs to attach
}

// ContactInput holds contact info for event creation.
type ContactInput struct {
	Email string `json:"email" binding:"required,email"`
	Name  string `json:"name"`
}

// CreateEvent creates a new event with contacts and sets up the Redis timer.
// Guarantees atomicity: if Redis timer setup fails, the DB event is rolled back.
func (s *EventService) CreateEvent(ctx context.Context, input CreateEventInput) (*models.Sequence, error) {
	now := time.Now()
	horizon := now.Add(time.Duration(input.DurationMS) * time.Millisecond)

	sequence := models.Sequence{
		EntityID:   input.UserID,
		Label:      input.Name,
		Payload:    input.Message,
		Horizon:    horizon,
		DurationMS: input.DurationMS,
	}

	// Create event with contacts in a transaction
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&sequence).Error; err != nil {
			return fmt.Errorf("failed to create event: %w", err)
		}

		for _, c := range input.Contacts {
			endpoint := models.Endpoint{
				SequenceID: sequence.ID,
				Signal:     c.Email,
				Alias:      c.Name,
			}
			if err := tx.Create(&endpoint).Error; err != nil {
				return fmt.Errorf("failed to create contact: %w", err)
			}
		}

		// Attach staged files to the event (if any)
		if err := s.fileService.AttachToEvent(tx, input.FileIDs, sequence.ID, input.UserID); err != nil {
			return fmt.Errorf("failed to attach files: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Set Redis key with expiry for timer trigger
	// Key format: timer:<event_id>
	// When this key expires, Redis keyspace notification fires the alert
	timerKey := fmt.Sprintf("timer:%s", sequence.ID.String())
	ttl := time.Until(horizon)
	if ttl < 0 {
		ttl = time.Millisecond // expire immediately if already past
	}
	if err := s.rdb.Set(ctx, timerKey, sequence.EntityID.String(), ttl).Err(); err != nil {
		// Compensating transaction: Redis failed, so remove the event from DB
		// to prevent orphaned events that the backup poller would fire
		log.Printf("Redis timer SET failed for event %s, rolling back DB: %v", sequence.ID, err)
		// Unattach staged files (set sequence_id back to NULL so they stay staged)
		s.db.Model(&models.Artifact{}).Where("sequence_id = ?", sequence.ID).Update("sequence_id", nil)
		s.db.Unscoped().Where("sequence_id = ?", sequence.ID).Delete(&models.Endpoint{})
		s.db.Unscoped().Where("id = ?", sequence.ID).Delete(&models.Sequence{})
		return nil, fmt.Errorf("failed to set timer — event not created, please retry")
	}

	// Reload with contacts
	s.db.Preload("Endpoints").Preload("Artifacts").First(&sequence, "id = ?", sequence.ID)

	return &sequence, nil
}

// GetEvent retrieves an event by ID, verifying ownership.
func (s *EventService) GetEvent(ctx context.Context, eventID, userID uuid.UUID) (*models.Sequence, error) {
	var sequence models.Sequence
	result := s.db.Preload("Endpoints").Preload("Artifacts").
		Where("id = ? AND entity_id = ?", eventID, userID).
		First(&sequence)
	if result.Error != nil {
		return nil, apperrors.ErrEventNotFound
	}
	return &sequence, nil
}

// ListEvents returns paginated events for a user.
func (s *EventService) ListEvents(ctx context.Context, userID uuid.UUID, page, limit int) ([]models.Sequence, int64, error) {
	var sequences []models.Sequence
	var total int64

	if err := s.db.Model(&models.Sequence{}).Where("entity_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	result := s.db.Preload("Endpoints").Preload("Artifacts").
		Where("entity_id = ?", userID).
		Order("created_at DESC").
		Offset(offset).Limit(limit).
		Find(&sequences)
	if result.Error != nil {
		return nil, 0, result.Error
	}
	return sequences, total, nil
}

// ListActiveEvents returns paginated active (non-terminated) events for a user.
func (s *EventService) ListActiveEvents(ctx context.Context, userID uuid.UUID, page, limit int) ([]models.Sequence, int64, error) {
	var sequences []models.Sequence
	var total int64

	if err := s.db.Model(&models.Sequence{}).Where("entity_id = ? AND terminated = false", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	result := s.db.Preload("Endpoints").Preload("Artifacts").
		Where("entity_id = ? AND terminated = false", userID).
		Order("horizon ASC").
		Offset(offset).Limit(limit).
		Find(&sequences)
	if result.Error != nil {
		return nil, 0, result.Error
	}
	return sequences, total, nil
}

// CheckIn marks an event as checked in and terminates it.
func (s *EventService) CheckIn(ctx context.Context, eventID, userID uuid.UUID) error {
	var sequence models.Sequence
	result := s.db.Where("id = ? AND entity_id = ? AND terminated = false", eventID, userID).First(&sequence)
	if result.Error != nil {
		return apperrors.ErrActiveEventNotFound
	}

	// Mark as checked in and terminated
	sequence.Pulse = true
	sequence.Terminated = true
	if err := s.db.Save(&sequence).Error; err != nil {
		return fmt.Errorf("failed to check in: %w", err)
	}

	// Remove Redis timer key (prevents expiry trigger)
	timerKey := fmt.Sprintf("timer:%s", eventID.String())
	s.rdb.Del(ctx, timerKey)

	return nil
}

// CancelEvent cancels an active event (soft delete).
func (s *EventService) CancelEvent(ctx context.Context, eventID, userID uuid.UUID) error {
	var sequence models.Sequence
	result := s.db.Where("id = ? AND entity_id = ? AND terminated = false", eventID, userID).First(&sequence)
	if result.Error != nil {
		return apperrors.ErrActiveEventNotFound
	}

	sequence.Terminated = true
	if err := s.db.Save(&sequence).Error; err != nil {
		return fmt.Errorf("failed to cancel event: %w", err)
	}

	// Remove Redis timer key
	timerKey := fmt.Sprintf("timer:%s", eventID.String())
	s.rdb.Del(ctx, timerKey)

	return nil
}

// GetExpiredUnfiredEvents returns events whose timer has passed but alerts haven't been sent.
// Used as a safety net backup polling sweep.
// Also recovers "stale claims" — events where fired=true but terminated=false for over 5 minutes
// (indicates a server crash during handleExpiration).
func (s *EventService) GetExpiredUnfiredEvents(ctx context.Context) ([]models.Sequence, error) {
	var sequences []models.Sequence

	// Case 1: normal missed expirations (never claimed)
	result := s.db.Preload("Endpoints").Preload("Artifacts").
		Where("terminated = false AND fired = false AND horizon <= ?", time.Now()).
		Find(&sequences)
	if result.Error != nil {
		return nil, result.Error
	}

	// Case 2: stale claims — claimed (fired=true) but never terminated, for over 5 minutes
	// This happens if the server crashed mid-handleExpiration
	staleThreshold := time.Now().Add(-5 * time.Minute)
	var stale []models.Sequence
	result = s.db.Preload("Endpoints").Preload("Artifacts").
		Where("terminated = false AND fired = true AND horizon <= ? AND updated_at <= ?", time.Now(), staleThreshold).
		Find(&stale)
	if result.Error != nil {
		return sequences, nil // return what we have, don't fail
	}

	// Reset stale claims so they can be re-claimed
	for i := range stale {
		log.Printf("Recovering stale claimed event %s (claimed but never completed)", stale[i].ID)
		s.db.Model(&models.Sequence{}).Where("id = ?", stale[i].ID).Update("fired", false)
		stale[i].Fired = false
		sequences = append(sequences, stale[i])
	}

	return sequences, nil
}

// MarkFired marks an event as fired (alerts sent) and terminated.
func (s *EventService) MarkFired(ctx context.Context, eventID uuid.UUID) error {
	return s.db.Model(&models.Sequence{}).
		Where("id = ?", eventID).
		Updates(map[string]interface{}{
			"fired":      true,
			"terminated": true,
		}).Error
}

// ClearUserTimers removes all Redis timers associated with the user's active events.
func (s *EventService) ClearUserTimers(ctx context.Context, userID uuid.UUID) error {
	var activeIDs []uuid.UUID
	if err := s.db.Model(&models.Sequence{}).Where("entity_id = ? AND terminated = false", userID).Pluck("id", &activeIDs).Error; err != nil {
		return fmt.Errorf("failed to fetch active events: %w", err)
	}

	for _, id := range activeIDs {
		timerKey := fmt.Sprintf("timer:%s", id.String())
		if err := s.rdb.Del(ctx, timerKey).Err(); err != nil {
			log.Printf("Warning: failed to delete redis timer for event %s: %v", id, err)
		}
	}
	return nil
}

// DeleteEventHistory deletes a terminated event and its associated data permanently from history.
func (s *EventService) DeleteEventHistory(ctx context.Context, eventID, userID uuid.UUID) error {
	var sequence models.Sequence
	result := s.db.Where("id = ? AND entity_id = ?", eventID, userID).First(&sequence)
	if result.Error != nil {
		return apperrors.ErrEventNotFound
	}

	if !sequence.Terminated {
		return fmt.Errorf("cannot delete an active event")
	}

	// Delete associated files from S3
	if err := s.fileService.DeleteEventFilesFromS3(ctx, eventID, userID); err != nil {
		return fmt.Errorf("failed to delete event files from S3: %w", err)
	}

	// Transactional DB cascade
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("sequence_id = ?", eventID).Delete(&models.Endpoint{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("sequence_id = ?", eventID).Delete(&models.Artifact{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("id = ?", eventID).Delete(&models.Sequence{}).Error; err != nil {
			return err
		}
		return nil
	})
}
