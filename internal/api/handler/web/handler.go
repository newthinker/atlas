// internal/api/handler/web/handler.go
package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed templates/*
var templateFS embed.FS

// Handler provides web UI handlers with template rendering
type Handler struct {
	templates *template.Template
}

// NewHandler creates a new web handler with templates loaded from the given directory.
// If templatesDir is empty, it falls back to embedded templates.
func NewHandler(templatesDir string) (*Handler, error) {
	var tmpl *template.Template
	var err error

	if templatesDir != "" {
		// Use filesystem templates from the provided directory
		tmpl, err = template.ParseGlob(filepath.Join(templatesDir, "*.html"))
		if err != nil {
			return nil, fmt.Errorf("parsing templates from directory: %w", err)
		}
	} else {
		// Use embedded templates as fallback
		subFS, err := fs.Sub(templateFS, "templates")
		if err != nil {
			return nil, fmt.Errorf("accessing embedded templates: %w", err)
		}

		tmpl, err = template.ParseFS(subFS, "*.html")
		if err != nil {
			return nil, fmt.Errorf("parsing embedded templates: %w", err)
		}
	}

	return &Handler{templates: tmpl}, nil
}

// NewHandlerWithFS creates a new web handler using a custom filesystem.
// This is useful for testing or custom template sources.
func NewHandlerWithFS(fsys fs.FS) (*Handler, error) {
	tmpl, err := template.ParseFS(fsys, "*.html")
	if err != nil {
		return nil, fmt.Errorf("parsing templates from fs: %w", err)
	}

	return &Handler{templates: tmpl}, nil
}

// TemplateFS returns the embedded template filesystem for external use.
func TemplateFS() fs.FS {
	subFS, err := fs.Sub(templateFS, "templates")
	if err != nil {
		// This should never happen with valid embed directive
		return templateFS
	}
	return subFS
}

// Ensure os import is used (for potential future directory checks)
var _ = os.DirFS
