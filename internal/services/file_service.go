package services

import (
	"context"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/sentinel-app/sentinel/internal/apperrors"
	"github.com/sentinel-app/sentinel/internal/config"
	"github.com/sentinel-app/sentinel/internal/models"
	"gorm.io/gorm"
)

// FileService handles file uploads to S3 and DB metadata.
type FileService struct {
	db       *gorm.DB
	s3Client *s3.Client
	cfg      *config.Config
}

// NewFileService creates a new FileService.
func NewFileService(db *gorm.DB, cfg *config.Config) *FileService {
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.AWSRegion),
		awsconfig.WithSharedConfigProfile(cfg.AWSProfile),
	)
	if err != nil {
		log.Fatalf("Failed to load AWS config for S3: %v", err)
	}

	return &FileService{
		db:       db,
		s3Client: s3.NewFromConfig(awsCfg),
		cfg:      cfg,
	}
}

// UploadStaged uploads a file to S3 as a staged (event-less) file owned by the user.
// Returns the artifact record. The file will be attached to an event in CreateEvent.
func (s *FileService) UploadStaged(ctx context.Context, userID uuid.UUID, fileName string, fileSize int64, mimeType string, body io.Reader) (*models.Artifact, error) {
	// Generate unique S3 key under staging folder
	ext := filepath.Ext(fileName)
	objectKey := fmt.Sprintf("staging/%s/%s%s", userID.String(), uuid.New().String(), ext)

	// Upload to S3
	_, err := s.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.cfg.S3Bucket),
		Key:         aws.String(objectKey),
		Body:        body,
		ContentType: aws.String(mimeType),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to upload to S3: %w", err)
	}

	// Create DB record with no event ID (staged)
	artifact := models.Artifact{
		EntityID:   userID,
		SequenceID: nil, // staged — no event yet
		AssetKey:   objectKey,
		FileName:   fileName,
		FileSize:   fileSize,
		MimeType:   mimeType,
	}
	if err := s.db.Create(&artifact).Error; err != nil {
		return nil, fmt.Errorf("failed to save file metadata: %w", err)
	}

	return &artifact, nil
}

// AttachToEvent assigns staged files to an event. Called inside event creation transaction.
// Validates that all file IDs belong to the user and are staged (not already attached).
func (s *FileService) AttachToEvent(tx *gorm.DB, fileIDs []uuid.UUID, eventID, userID uuid.UUID) error {
	if len(fileIDs) == 0 {
		return nil
	}

	// Verify all files exist, belong to user, and are staged
	var files []models.Artifact
	result := tx.Where("id IN ? AND entity_id = ? AND sequence_id IS NULL", fileIDs, userID).Find(&files)
	if result.Error != nil {
		return fmt.Errorf("failed to look up files: %w", result.Error)
	}
	if len(files) != len(fileIDs) {
		return apperrors.ErrFileAttachment
	}

	// Attach all to the event
	if err := tx.Model(&models.Artifact{}).
		Where("id IN ? AND entity_id = ? AND sequence_id IS NULL", fileIDs, userID).
		Update("sequence_id", eventID).Error; err != nil {
		return fmt.Errorf("failed to attach files to event: %w", err)
	}

	return nil
}

// ListStagedFiles returns all staged (unattached) files for a user.
func (s *FileService) ListStagedFiles(ctx context.Context, userID uuid.UUID) ([]models.Artifact, error) {
	var files []models.Artifact
	result := s.db.Where("entity_id = ? AND sequence_id IS NULL", userID).
		Order("created_at ASC").
		Find(&files)
	if result.Error != nil {
		return nil, result.Error
	}
	return files, nil
}

// DeleteStagedFile deletes a staged file from S3 and DB.
func (s *FileService) DeleteStagedFile(ctx context.Context, fileID, userID uuid.UUID) error {
	var artifact models.Artifact
	result := s.db.Where("id = ? AND entity_id = ? AND sequence_id IS NULL", fileID, userID).First(&artifact)
	if result.Error != nil {
		return apperrors.ErrStagedFileNotFound
	}

	// Delete from S3
	_, err := s.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.cfg.S3Bucket),
		Key:    aws.String(artifact.AssetKey),
	})
	if err != nil {
		log.Printf("Warning: failed to delete S3 object %s: %v", artifact.AssetKey, err)
	}

	// Delete DB record
	if err := s.db.Delete(&artifact).Error; err != nil {
		return fmt.Errorf("failed to delete file record: %w", err)
	}

	return nil
}

// CleanupStaleStagedFiles deletes staged files older than 24 hours.
// Called by the backup poller as a cleanup sweep.
func (s *FileService) CleanupStaleStagedFiles(ctx context.Context) {
	threshold := time.Now().Add(-24 * time.Hour)
	var stale []models.Artifact
	s.db.Where("sequence_id IS NULL AND created_at <= ?", threshold).Find(&stale)

	for _, artifact := range stale {
		log.Printf("Cleaning up stale staged file %s (uploaded %s)", artifact.ID, artifact.CreatedAt)
		// Delete from S3
		s.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(s.cfg.S3Bucket),
			Key:    aws.String(artifact.AssetKey),
		})
		// Delete DB record
		s.db.Delete(&artifact)
	}

	if len(stale) > 0 {
		log.Printf("Cleaned up %d stale staged files", len(stale))
	}
}

// GetFileURL generates a pre-signed URL for downloading a file from S3.
func (s *FileService) GetFileURL(ctx context.Context, objectKey string) (string, error) {
	presignClient := s3.NewPresignClient(s.s3Client)

	req, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.cfg.S3Bucket),
		Key:    aws.String(objectKey),
	}, s3.WithPresignExpires(15*time.Minute))
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return req.URL, nil
}

// DownloadFile downloads a file from S3 (used for email attachments).
func (s *FileService) DownloadFile(ctx context.Context, objectKey string) ([]byte, error) {
	result, err := s.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.cfg.S3Bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to download from S3: %w", err)
	}
	defer result.Body.Close()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read S3 object: %w", err)
	}

	return data, nil
}
