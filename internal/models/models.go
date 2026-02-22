// Package models defines the database schema using GORM.
//
// TABLE NAME OBFUSCATION MAPPING (security measure):
//
//	entities     = users
//	sequences    = events
//	endpoints    = contacts (per-event recipients)
//	coordinates  = location updates
//	artifacts    = file attachments
//	connections  = saved contacts (user address book)
//	tokens       = OTP/auth tokens (stored in Redis, not DB)
//
// COLUMN NAME OBFUSCATION:
//
//	entity_id    = user_id
//	sequence_id  = event_id
//	signal       = email address
//	pulse        = check-in status
//	horizon      = timer expiry timestamp
//	payload      = custom message
//	vector       = GPS coordinates
//	asset_key    = S3 object key
package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Entity represents a user (obfuscated table: "entities").
type Entity struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	Signal    string    `gorm:"uniqueIndex;column:signal;not null" json:"email"` // email address
	Alias     string    `gorm:"column:alias" json:"name"`                        // display name
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (Entity) TableName() string { return "entities" }

func (e *Entity) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}

// Sequence represents an event/timer (obfuscated table: "sequences").
type Sequence struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	EntityID   uuid.UUID      `gorm:"type:uuid;index;column:entity_id;not null" json:"user_id"` // owner
	Label      string         `gorm:"column:label" json:"name"`                                 // event display name
	Payload    string         `gorm:"column:payload;type:text" json:"message"`                  // custom message
	Horizon    time.Time      `gorm:"column:horizon;not null" json:"expires_at"`                // timer expiry
	DurationMS int64          `gorm:"column:duration_ms;not null" json:"duration_ms"`           // original duration in milliseconds
	Pulse        bool           `gorm:"column:pulse;default:false" json:"checked_in"`             // true = user checked in
	Terminated   bool           `gorm:"column:terminated;default:false" json:"terminated"`        // true = event is done (checked in or fired)
	Fired        bool           `gorm:"column:fired;default:false" json:"fired"`                  // true = alert emails were sent
	FireAttempts int            `gorm:"column:fire_attempts;default:0" json:"fire_attempts"`      // number of times alert was attempted
	NextRetryAt  *time.Time     `gorm:"column:next_retry_at" json:"next_retry_at,omitempty"`      // backoff time for retry
	CreatedAt    time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt    time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`

	// Relations
	Endpoints []Endpoint `gorm:"foreignKey:SequenceID" json:"contacts,omitempty"`
	Artifacts []Artifact `gorm:"foreignKey:SequenceID" json:"attachments,omitempty"`
}

func (Sequence) TableName() string { return "sequences" }

func (s *Sequence) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

// Endpoint represents a contact for an event (obfuscated table: "endpoints").
type Endpoint struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	SequenceID uuid.UUID `gorm:"type:uuid;index;column:sequence_id;not null" json:"event_id"`
	Signal     string    `gorm:"column:signal;not null" json:"email"` // contact email
	Alias      string    `gorm:"column:alias" json:"name"`            // contact name
	CreatedAt  time.Time `gorm:"column:created_at" json:"created_at"`
}

func (Endpoint) TableName() string { return "endpoints" }

func (e *Endpoint) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}

// Coordinate stores the latest GPS location for a user (obfuscated table: "coordinates").
type Coordinate struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	EntityID  uuid.UUID `gorm:"type:uuid;uniqueIndex;column:entity_id;not null" json:"user_id"`
	Lat       float64   `gorm:"column:lat;not null" json:"lat"`
	Lng       float64   `gorm:"column:lng;not null" json:"lng"`
	Accuracy  float64   `gorm:"column:accuracy" json:"accuracy"` // meters
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (Coordinate) TableName() string { return "coordinates" }

func (c *Coordinate) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

// Artifact stores metadata for an uploaded file (obfuscated table: "artifacts").
// Files are first uploaded as staged (no event), then attached to an event at creation.
type Artifact struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	EntityID   uuid.UUID  `gorm:"type:uuid;index;column:entity_id;not null" json:"user_id"` // owner
	SequenceID *uuid.UUID `gorm:"type:uuid;index;column:sequence_id" json:"event_id"`       // null = staged
	AssetKey   string     `gorm:"column:asset_key;not null" json:"key"`                     // S3 object key
	FileName   string     `gorm:"column:file_name;not null" json:"filename"`
	FileSize   int64      `gorm:"column:file_size" json:"file_size"`
	MimeType   string     `gorm:"column:mime_type" json:"mime_type"`
	CreatedAt  time.Time  `gorm:"column:created_at" json:"created_at"`
}

func (Artifact) TableName() string { return "artifacts" }

func (a *Artifact) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

// Connection represents a saved contact in a user's address book (obfuscated table: "connections").
type Connection struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	EntityID  uuid.UUID `gorm:"type:uuid;index;column:entity_id;not null" json:"user_id"`
	Signal    string    `gorm:"column:signal;not null" json:"email"` // contact email
	Alias     string    `gorm:"column:alias" json:"name"`            // contact display name
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (Connection) TableName() string { return "connections" }

func (c *Connection) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}
