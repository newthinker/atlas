// internal/storage/archive/s3_test.go
package archive

import (
	"strings"
	"testing"
)

func TestS3Storage_ImplementsStorage(t *testing.T) {
	var _ Storage = (*S3Storage)(nil)
}

func TestS3Config_Key(t *testing.T) {
	tests := []struct {
		prefix string
		path   string
		want   string
	}{
		{"", "file.txt", "file.txt"},
		{"archive", "file.txt", "archive/file.txt"},
		{"archive/", "file.txt", "archive/file.txt"},
	}

	for _, tt := range tests {
		s := &S3Storage{prefix: strings.TrimSuffix(tt.prefix, "/")}
		got := s.key(tt.path)
		if got != tt.want {
			t.Errorf("key(%q) with prefix %q = %q, want %q", tt.path, tt.prefix, got, tt.want)
		}
	}
}
