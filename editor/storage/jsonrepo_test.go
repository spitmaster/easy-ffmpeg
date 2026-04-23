package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"easy-ffmpeg/editor/domain"
	"easy-ffmpeg/editor/ports"
)

func sampleProject(id, name string, t time.Time) *domain.Project {
	return &domain.Project{
		SchemaVersion: domain.SchemaVersion,
		ID:            id,
		Name:          name,
		CreatedAt:     t,
		UpdatedAt:     t,
		Source: domain.Source{
			Path:     "C:/videos/a.mp4",
			Duration: 60,
			Width:    1920,
			Height:   1080,
		},
		VideoClips: []domain.Clip{{ID: "v1", SourceStart: 0, SourceEnd: 60}},
		AudioClips: []domain.Clip{{ID: "a1", SourceStart: 0, SourceEnd: 60}},
	}
}

func TestJSONRepoRoundTrip(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewJSONRepo(dir)
	if err != nil {
		t.Fatalf("NewJSONRepo: %v", err)
	}
	ctx := context.Background()
	now := time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC)
	p := sampleProject("abcd1234", "My Edit", now)

	if err := repo.Save(ctx, p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "My Edit" || got.Source.Path != "C:/videos/a.mp4" {
		t.Errorf("unexpected got: %+v", got)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != "abcd1234" {
		t.Errorf("List = %+v", list)
	}
}

func TestJSONRepoDelete(t *testing.T) {
	repo, _ := NewJSONRepo(t.TempDir())
	ctx := context.Background()
	p := sampleProject("xx", "n", time.Now())
	_ = repo.Save(ctx, p)

	if err := repo.Delete(ctx, "xx"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.Get(ctx, "xx"); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
	if err := repo.Delete(ctx, "xx"); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("double-delete: want ErrNotFound, got %v", err)
	}
}

func TestJSONRepoListOrderedByUpdate(t *testing.T) {
	repo, _ := NewJSONRepo(t.TempDir())
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	a := sampleProject("a", "old", t0)
	b := sampleProject("b", "new", t0.Add(time.Hour))
	_ = repo.Save(ctx, a)
	_ = repo.Save(ctx, b)

	list, _ := repo.List(ctx)
	if len(list) != 2 {
		t.Fatalf("len=%d", len(list))
	}
	if list[0].ID != "b" {
		t.Errorf("expected newest-updated first, got %v first", list[0].ID)
	}
}

func TestJSONRepoRebuildsIndexOnCorruption(t *testing.T) {
	dir := t.TempDir()
	repo, _ := NewJSONRepo(dir)
	ctx := context.Background()
	p := sampleProject("rec1", "r", time.Now())
	_ = repo.Save(ctx, p)

	// corrupt the index
	if err := os.WriteFile(filepath.Join(dir, "index.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	// new repo should rebuild from files
	repo2, err := NewJSONRepo(dir)
	if err != nil {
		t.Fatalf("NewJSONRepo after corruption: %v", err)
	}
	if _, err := repo2.Get(ctx, "rec1"); err != nil {
		t.Errorf("rebuilt repo should find project: %v", err)
	}
}

func TestJSONRepoSaveUpdatesExisting(t *testing.T) {
	repo, _ := NewJSONRepo(t.TempDir())
	ctx := context.Background()
	now := time.Now()
	p := sampleProject("same", "first", now)
	_ = repo.Save(ctx, p)

	p.Name = "second"
	p.UpdatedAt = now.Add(time.Minute)
	if err := repo.Save(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.Get(ctx, "same")
	if got.Name != "second" {
		t.Errorf("update didn't persist: name = %q", got.Name)
	}
	list, _ := repo.List(ctx)
	if len(list) != 1 {
		t.Errorf("update should not duplicate, got %d entries", len(list))
	}
}
