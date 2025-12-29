// internal/storage/archive/interface.go
package archive

import "context"

// Storage defines the interface for cold/archive storage backends
type Storage interface {
	// Write stores data at the given path
	Write(ctx context.Context, path string, data []byte) error

	// Read retrieves data from the given path
	Read(ctx context.Context, path string) ([]byte, error)

	// List returns all paths matching the prefix
	List(ctx context.Context, prefix string) ([]string, error)

	// Delete removes the data at the given path
	Delete(ctx context.Context, path string) error

	// Exists checks if data exists at the given path
	Exists(ctx context.Context, path string) (bool, error)
}
