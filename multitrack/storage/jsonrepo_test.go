package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"easy-ffmpeg/multitrack/domain"
	"easy-ffmpeg/multitrack/ports"
)

func newRepo(t *testing.T) *JSONRepo {
	t.Helper()
	r, err := NewJSONRepo(t.TempDir())
	if err != nil {
		t.Fatalf("NewJSONRepo: %v", err)
	}
	return r
}

func newProj(id, name string, srcCount int) *domain.Project {
	now := time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC)
	p := domain.NewProject(id, name, now)
	for i := 0; i < srcCount; i++ {
		p.Sources = append(p.Sources, domain.Source{
			ID:       "s" + string(rune('1'+i)),
			Path:     "/tmp/x.mp4",
			Kind:     domain.SourceVideo,
			Duration: 10,
		})
	}
	return p
}

func TestSaveListGet(t *testing.T) {
	r := newRepo(t)
	ctx := context.Background()
	p := newProj("p1", "Demo", 2)
	if err := r.Save(ctx, p); err != nil {
		t.Fatalf("Save: %v", err)
	}
	rows, err := r.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "p1" || rows[0].SourceCount != 2 {
		t.Fatalf("List rows mismatch: %+v", rows)
	}
	got, err := r.Get(ctx, "p1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Demo" || len(got.Sources) != 2 {
		t.Fatalf("Get round-trip mismatch: %+v", got)
	}
}

func TestDelete(t *testing.T) {
	r := newRepo(t)
	ctx := context.Background()
	if err := r.Save(ctx, newProj("p1", "Demo", 0)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := r.Delete(ctx, "p1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.Get(ctx, "p1"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Get after Delete: want ErrNotFound, got %v", err)
	}
	if err := r.Delete(ctx, "p1"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("double Delete: want ErrNotFound, got %v", err)
	}
}

func TestRebuildIndexFromDisk(t *testing.T) {
	dir := t.TempDir()
	// First instance creates the file + index.
	r1, err := NewJSONRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := r1.Save(context.Background(), newProj("p1", "Demo", 0)); err != nil {
		t.Fatal(err)
	}
	// Nuke the index — second instance should rebuild from the *.json files.
	if err := os.Remove(filepath.Join(dir, "index.json")); err != nil {
		t.Fatal(err)
	}
	r2, err := NewJSONRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := r2.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "p1" {
		t.Fatalf("rebuild missed entry: %+v", rows)
	}
}

func TestForeignKindFilesIgnored(t *testing.T) {
	dir := t.TempDir()
	// Drop a single-video shaped file in the dir and ensure rebuild skips it.
	bad := filepath.Join(dir, "alien.json")
	if err := os.WriteFile(bad, []byte(`{"id":"sv1","kind":"single-video","schemaVersion":3}`), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := NewJSONRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := r.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("foreign-kind file leaked into list: %+v", rows)
	}
}
