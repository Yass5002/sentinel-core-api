// Package apperrors defines sentinel errors for the Sentinel application.
// These are used for type-safe error dispatch in handler layers instead of
// fragile string comparison.
package apperrors

import "errors"

// ErrContactDuplicate is returned when a contact with the same email already exists.
var ErrContactDuplicate = errors.New("contact with this email already exists")

// ErrContactNotFound is returned when a contact cannot be found.
var ErrContactNotFound = errors.New("contact not found")

// ErrEventNotFound is returned when an event cannot be found or is not owned by user.
var ErrEventNotFound = errors.New("event not found")

// ErrActiveEventNotFound is returned when an active (non-terminated) event is required but not found.
var ErrActiveEventNotFound = errors.New("active event not found")

// ErrStagedFileNotFound is returned when a staged file cannot be found.
var ErrStagedFileNotFound = errors.New("staged file not found")

// ErrUserNotFound is returned when a user profile is not found.
var ErrUserNotFound = errors.New("user not found")

// ErrLocationNotFound is returned when no location data exists for a user.
var ErrLocationNotFound = errors.New("location not found")

// ErrFileAttachment is returned when file IDs are invalid or not attachable.
var ErrFileAttachment = errors.New("one or more file IDs are invalid, not owned by you, or already attached to an event")
