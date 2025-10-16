package configstore

import (
	"context"

	"labmonitor/internal/config"
)

// Store provides access to the Lab Monitor configuration.
// Implementations should handle optimistic concurrency via a version token.
type Store interface {
	// Get retrieves the current configuration and an opaque version token.
	Get(ctx context.Context) (config.Config, string, error)
	// Put persists the supplied configuration if the supplied version still matches.
	// Implementations should return ErrConflict if the version no longer matches.
	Put(ctx context.Context, cfg config.Config, version string) error
}

// ErrConflict indicates an optimistic concurrency failure.
type ErrConflict struct {
	Message string
}

func (e ErrConflict) Error() string {
	if e.Message == "" {
		return "config version conflict"
	}
	return e.Message
}
