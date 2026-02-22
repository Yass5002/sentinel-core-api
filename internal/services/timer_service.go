package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"html/template"
	"log"
	"mime/multipart"
	"net/textproto"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sentinel-app/sentinel/internal/config"
	"github.com/sentinel-app/sentinel/internal/models"
	"github.com/sentinel-app/sentinel/internal/templates"
	"gorm.io/gorm"
)

// emailAttachment holds file data for sending with alert emails.
type emailAttachment struct {
	filename string
	mimeType string
	data     []byte
}

// TimerService handles timer expiration via Redis keyspace notifications
// and a backup polling sweep.
type TimerService struct {
	db       *gorm.DB
	rdb      *redis.Client
	cfg      *config.Config
	event    *EventService
	location *LocationService
	file     *FileService
	email    *EmailService
	tmpl     *templates.Engine
}

// NewTimerService creates a new TimerService.
func NewTimerService(
	db *gorm.DB,
	rdb *redis.Client,
	cfg *config.Config,
	event *EventService,
	location *LocationService,
	file *FileService,
	email *EmailService,
	tmpl *templates.Engine,
) *TimerService {
	return &TimerService{
		db:       db,
		rdb:      rdb,
		cfg:      cfg,
		event:    event,
		location: location,
		file:     file,
		email:    email,
		tmpl:     tmpl,
	}
}

// StartKeyspaceListener subscribes to Redis keyspace notifications for expired keys.
// When a timer key expires, it triggers alert emails.
func (s *TimerService) StartKeyspaceListener(ctx context.Context) {
	// Try to enable keyspace notifications. If this fails (e.g., on managed Redis
	// like ElastiCache), it must be configured via the cluster's parameter group.
	if err := s.rdb.ConfigSet(ctx, "notify-keyspace-events", "Ex").Err(); err != nil {
		log.Printf("Warning: could not set Redis keyspace notifications (configure via parameter group if using managed Redis): %v", err)
	}

	// Subscribe to expired key events on db 0
	pubsub := s.rdb.PSubscribe(ctx, "__keyevent@0__:expired")

	go func() {
		log.Println("Timer keyspace listener started")
		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				pubsub.Close()
				log.Println("Timer keyspace listener stopped")
				return
			case msg := <-ch:
				if msg == nil {
					continue
				}
				// Key format: timer:<event_id>
				if strings.HasPrefix(msg.Payload, "timer:") {
					eventIDStr := strings.TrimPrefix(msg.Payload, "timer:")
					eventID, err := uuid.Parse(eventIDStr)
					if err != nil {
						log.Printf("Invalid event ID from expired key: %s", eventIDStr)
						continue
					}
					log.Printf("Timer expired for event: %s", eventID)
					go s.handleExpiration(context.Background(), eventID)
				}
			}
		}
	}()
}

// StartBackupPoller runs a periodic sweep to catch any missed expirations.
// Safety net in case Redis keyspace notification is missed.
func (s *TimerService) StartBackupPoller(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		log.Printf("Backup timer poller started (interval: %s)", interval)
		for {
			select {
			case <-ctx.Done():
				log.Println("Backup timer poller stopped")
				return
			case <-ticker.C:
				s.sweepExpired(ctx)
			}
		}
	}()
}

// sweepExpired finds expired, unfired events and triggers alerts.
// Also cleans up stale staged files older than 24h.
func (s *TimerService) sweepExpired(ctx context.Context) {
	events, err := s.event.GetExpiredUnfiredEvents(ctx)
	if err != nil {
		log.Printf("Backup poller error: %v", err)
		return
	}

	for _, evt := range events {
		log.Printf("Backup poller caught expired event: %s", evt.ID)
		s.handleExpiration(ctx, evt.ID)
	}

	// Clean up stale staged files (uploaded but never attached to an event)
	s.file.CleanupStaleStagedFiles(ctx)
}

// handleExpiration processes a timer expiration: sends alert emails to all contacts.
// Uses atomic DB claim to prevent duplicate processing by concurrent goroutines.
func (s *TimerService) handleExpiration(ctx context.Context, eventID uuid.UUID) {
	// Atomically claim this event for processing.
	// Only one goroutine (keyspace listener OR backup poller) can win this race.
	claimResult := s.db.Model(&models.Sequence{}).
		Where("id = ? AND terminated = false AND fired = false", eventID).
		Update("fired", true)
	if claimResult.Error != nil {
		log.Printf("Error claiming event %s: %v", eventID, claimResult.Error)
		return
	}
	if claimResult.RowsAffected == 0 {
		// Another goroutine already claimed it, or it was checked in / cancelled
		log.Printf("Event %s already claimed or terminated, skipping", eventID)
		return
	}

	log.Printf("Claimed event %s for alert processing", eventID)

	// Load event with contacts and attachments
	var sequence models.Sequence
	result := s.db.Preload("Endpoints").Preload("Artifacts").
		Where("id = ?", eventID).
		First(&sequence)
	if result.Error != nil {
		log.Printf("Error loading claimed event %s: %v", eventID, result.Error)
		// Unclaim so backup poller can retry
		s.db.Model(&models.Sequence{}).Where("id = ?", eventID).Update("fired", false)
		return
	}

	// Get user's last known location
	var locationText string
	coord, err := s.location.GetLocation(ctx, sequence.EntityID)
	if err == nil {
		locationText = fmt.Sprintf(
			"Last known location: %.6f, %.6f (accuracy: %.0fm)\nGoogle Maps: https://maps.google.com/?q=%.6f,%.6f",
			coord.Lat, coord.Lng, coord.Accuracy, coord.Lat, coord.Lng,
		)
	} else {
		locationText = "Last known location: unavailable"
	}

	// Get user email for context
	var entity models.Entity
	s.db.First(&entity, "id = ?", sequence.EntityID)

	// Download attachments from S3
	var attachments []emailAttachment

	for _, artifact := range sequence.Artifacts {
		data, err := s.file.DownloadFile(ctx, artifact.AssetKey)
		if err != nil {
			log.Printf("Failed to download attachment %s: %v", artifact.AssetKey, err)
			continue
		}
		attachments = append(attachments, emailAttachment{
			filename: artifact.FileName,
			mimeType: artifact.MimeType,
			data:     data,
		})
	}

	// Build and send alert to each contact
	subject := "SENTINEL ALERT — Dead Man's Switch Triggered"
	if sequence.Label != "" {
		subject = fmt.Sprintf("SENTINEL ALERT — %s", sequence.Label)
	}

	// Build attachment metadata for the template
	var attachmentInfos []templates.AttachmentInfo
	for _, artifact := range sequence.Artifacts {
		attachmentInfos = append(attachmentInfos, templates.AttachmentInfo{
			Filename: artifact.FileName,
			Size:     formatFileSize(artifact.FileSize),
			MimeType: artifact.MimeType,
		})
	}

	htmlBody, err := s.tmpl.RenderAlert(templates.AlertData{
		UserName:     entity.Alias,
		UserEmail:    entity.Signal,
		Message:      sequence.Payload,
		Location:     template.HTML(strings.ReplaceAll(locationText, "\n", "<br>")),
		EventCreated: sequence.CreatedAt.Format(time.RFC3339),
		TimerExpired: sequence.Horizon.Format(time.RFC3339),
		Attachments:  attachmentInfos,
	})
	if err != nil {
		log.Printf("Failed to render alert email: %v", err)
		return
	}

	successCount := 0
	for _, endpoint := range sequence.Endpoints {

		var sendErr error
		// Retry up to 3 times per contact
		for attempt := 1; attempt <= 3; attempt++ {
			if len(attachments) > 0 {
				// Send raw email with attachments
				rawMsg, err := buildRawEmail(
					s.cfg.SESFromAlert,
					endpoint.Signal,
					subject,
					htmlBody,
					attachments,
				)
				if err != nil {
					sendErr = fmt.Errorf("build raw email: %w", err)
					break // don't retry build failures
				}
				sendErr = s.email.SendAlertRaw(ctx, rawMsg, s.cfg.SESFromAlert, []string{endpoint.Signal})
			} else {
				// Simple email without attachments
				sendErr = s.email.SendAlert(ctx, endpoint.Signal, subject, htmlBody)
			}

			if sendErr == nil {
				break // success
			}
			log.Printf("Attempt %d/%d failed sending alert to %s: %v", attempt, 3, endpoint.Signal, sendErr)
			if attempt < 3 {
				time.Sleep(time.Duration(attempt) * 2 * time.Second) // backoff: 2s, 4s
			}
		}

		if sendErr != nil {
			log.Printf("FAILED all 3 attempts sending alert to %s for event %s: %v", endpoint.Signal, eventID, sendErr)
			continue
		}

		successCount++
		log.Printf("Alert sent to %s for event %s", endpoint.Signal, eventID)
	}

	if successCount == 0 && len(sequence.Endpoints) > 0 {
		// All sends failed — unclaim so backup poller can retry later
		log.Printf("ALL alerts failed for event %s — unclaiming for retry", eventID)
		s.db.Model(&models.Sequence{}).Where("id = ?", eventID).
			Updates(map[string]interface{}{"fired": false})
		return
	}

	// Mark event as terminated (fired was already set during claim)
	s.db.Model(&models.Sequence{}).Where("id = ?", eventID).
		Update("terminated", true)

	log.Printf("Event %s fully processed — %d/%d alerts sent", eventID, successCount, len(sequence.Endpoints))
}

// buildRawEmail constructs a MIME multipart email with attachments.
func buildRawEmail(from, to, subject, htmlBody string, attachments []emailAttachment) ([]byte, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Headers
	boundary := writer.Boundary()
	buf.Reset()

	fmt.Fprintf(&buf, "From: %s\r\n", from)
	fmt.Fprintf(&buf, "To: %s\r\n", to)
	fmt.Fprintf(&buf, "Subject: %s\r\n", subject)
	fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&buf, "Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", boundary)

	// HTML body part
	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	fmt.Fprintf(&buf, "Content-Type: text/html; charset=UTF-8\r\n")
	fmt.Fprintf(&buf, "Content-Transfer-Encoding: 7bit\r\n\r\n")
	fmt.Fprintf(&buf, "%s\r\n\r\n", htmlBody)

	// Attachment parts
	for _, att := range attachments {
		fmt.Fprintf(&buf, "--%s\r\n", boundary)
		h := make(textproto.MIMEHeader)
		h.Set("Content-Type", att.mimeType)
		h.Set("Content-Transfer-Encoding", "base64")
		h.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", att.filename))
		for k, v := range h {
			fmt.Fprintf(&buf, "%s: %s\r\n", k, v[0])
		}
		fmt.Fprintf(&buf, "\r\n")

		encoded := base64.StdEncoding.EncodeToString(att.data)
		// Write in 76-char lines per RFC 2045
		for i := 0; i < len(encoded); i += 76 {
			end := i + 76
			if end > len(encoded) {
				end = len(encoded)
			}
			fmt.Fprintf(&buf, "%s\r\n", encoded[i:end])
		}
	}

	fmt.Fprintf(&buf, "--%s--\r\n", boundary)

	return buf.Bytes(), nil
}

// formatFileSize returns a human-readable file size string.
func formatFileSize(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
	)
	switch {
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.0f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
