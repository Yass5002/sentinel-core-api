package handlers

import "github.com/google/uuid"

// parseUUID is a shared helper to parse UUID strings in handler endpoints.
func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}
