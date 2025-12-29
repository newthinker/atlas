// internal/storage/archive/localfs_test.go
package archive

import (
	"context"
	"testing"
)

func TestLocalFS_ImplementsStorage(t *testing.T) {
	var _ Storage = (*LocalFS)(nil)
}

func TestLocalFS_WriteRead(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewLocalFS(dir)
	if err != nil {
		t.Fatalf("NewLocalFS: %v", err)
	}

	ctx := context.Background()
	data := []byte("test data")

	if err := fs.Write(ctx, "test/file.txt", data); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := fs.Read(ctx, "test/file.txt")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if string(got) != string(data) {
		t.Errorf("got %q, want %q", got, data)
	}
}

func TestLocalFS_Exists(t *testing.T) {
	dir := t.TempDir()
	fs, _ := NewLocalFS(dir)
	ctx := context.Background()

	exists, _ := fs.Exists(ctx, "nonexistent.txt")
	if exists {
		t.Error("expected false for nonexistent file")
	}

	fs.Write(ctx, "exists.txt", []byte("data"))
	exists, _ = fs.Exists(ctx, "exists.txt")
	if !exists {
		t.Error("expected true for existing file")
	}
}

func TestLocalFS_List(t *testing.T) {
	dir := t.TempDir()
	fs, _ := NewLocalFS(dir)
	ctx := context.Background()

	fs.Write(ctx, "data/2024/01/a.txt", []byte("a"))
	fs.Write(ctx, "data/2024/01/b.txt", []byte("b"))
	fs.Write(ctx, "data/2024/02/c.txt", []byte("c"))

	paths, err := fs.List(ctx, "data/2024/01")
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(paths) != 2 {
		t.Errorf("expected 2 paths, got %d", len(paths))
	}
}

func TestLocalFS_Delete(t *testing.T) {
	dir := t.TempDir()
	fs, _ := NewLocalFS(dir)
	ctx := context.Background()

	fs.Write(ctx, "delete.txt", []byte("data"))
	fs.Delete(ctx, "delete.txt")

	exists, _ := fs.Exists(ctx, "delete.txt")
	if exists {
		t.Error("file should be deleted")
	}
}
