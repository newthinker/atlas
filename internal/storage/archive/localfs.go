// internal/storage/archive/localfs.go
package archive

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// LocalFS implements Storage for local filesystem
type LocalFS struct {
	basePath string
}

// NewLocalFS creates a new LocalFS storage
func NewLocalFS(basePath string) (*LocalFS, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("creating base path: %w", err)
	}
	return &LocalFS{basePath: basePath}, nil
}

func (l *LocalFS) fullPath(path string) string {
	return filepath.Join(l.basePath, path)
}

func (l *LocalFS) Write(ctx context.Context, path string, data []byte) error {
	fullPath := l.fullPath(path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("creating directories: %w", err)
	}
	return os.WriteFile(fullPath, data, 0644)
}

func (l *LocalFS) Read(ctx context.Context, path string) ([]byte, error) {
	return os.ReadFile(l.fullPath(path))
}

func (l *LocalFS) List(ctx context.Context, prefix string) ([]string, error) {
	var paths []string
	searchPath := l.fullPath(prefix)

	err := filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relPath, _ := filepath.Rel(l.basePath, path)
			paths = append(paths, relPath)
		}
		return nil
	})

	if os.IsNotExist(err) {
		return []string{}, nil
	}
	return paths, err
}

func (l *LocalFS) Delete(ctx context.Context, path string) error {
	return os.Remove(l.fullPath(path))
}

func (l *LocalFS) Exists(ctx context.Context, path string) (bool, error) {
	_, err := os.Stat(l.fullPath(path))
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
