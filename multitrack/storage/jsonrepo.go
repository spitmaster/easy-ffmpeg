// Package storage provides concrete implementations of
// multitrack/ports.ProjectRepository. Mirrors editor/storage in shape and
// guarantees: atomic writes, sidecar index for fast listing, self-healing
// rebuild when the index is missing or malformed.
package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"easy-ffmpeg/multitrack/domain"
	"easy-ffmpeg/multitrack/ports"
)

// JSONRepo stores each multitrack project as one JSON file under dir,
// plus a sidecar index.json for fast listing. File layout:
//
//	<dir>/index.json
//	<dir>/<createdAt>_<id>.json
//
// It is safe for concurrent use; a single RWMutex guards both the index
// cache and the on-disk files. That's coarse but appropriate for a local
// single-user tool.
type JSONRepo struct {
	dir       string
	indexPath string

	mu    sync.RWMutex
	index map[string]indexEntry // id -> entry
}

// indexEntry is the persisted index row plus the resolved filename.
type indexEntry struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	SourceCount int       `json:"sourceCount"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	FileName    string    `json:"fileName"`
}

// NewJSONRepo creates or opens a JSONRepo rooted at dir. It creates the
// directory if missing and rebuilds the index by scanning *.json when the
// index file is absent or malformed.
func NewJSONRepo(dir string) (*JSONRepo, error) {
	if dir == "" {
		return nil, errors.New("multitrack/jsonrepo: dir is empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("multitrack/jsonrepo: mkdir: %w", err)
	}
	r := &JSONRepo{
		dir:       dir,
		indexPath: filepath.Join(dir, "index.json"),
		index:     make(map[string]indexEntry),
	}
	if err := r.loadOrRebuildIndex(); err != nil {
		return nil, err
	}
	return r, nil
}

// List returns all summaries, newest-updated first.
func (r *JSONRepo) List(_ context.Context) ([]ports.ProjectSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ports.ProjectSummary, 0, len(r.index))
	for _, e := range r.index {
		out = append(out, ports.ProjectSummary{
			ID:          e.ID,
			Name:        e.Name,
			SourceCount: e.SourceCount,
			CreatedAt:   e.CreatedAt.Format(time.RFC3339),
			UpdatedAt:   e.UpdatedAt.Format(time.RFC3339),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}

// Get reads the full project JSON from disk.
func (r *JSONRepo) Get(_ context.Context, id string) (*domain.Project, error) {
	r.mu.RLock()
	entry, ok := r.index[id]
	r.mu.RUnlock()
	if !ok {
		return nil, ports.ErrNotFound
	}
	full := filepath.Join(r.dir, entry.FileName)
	data, err := os.ReadFile(full)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ports.ErrNotFound
		}
		return nil, fmt.Errorf("multitrack/jsonrepo: read %s: %w", entry.FileName, err)
	}
	var p domain.Project
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("multitrack/jsonrepo: parse %s: %w", entry.FileName, err)
	}
	p.Migrate()
	return &p, nil
}

// Save writes the project atomically (temp file + rename) and updates the
// index. If the project is new, a filename is generated from CreatedAt + id.
func (r *JSONRepo) Save(_ context.Context, p *domain.Project) error {
	if p == nil {
		return errors.New("multitrack/jsonrepo: project is nil")
	}
	if p.ID == "" {
		return errors.New("multitrack/jsonrepo: project id is empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.index[p.ID]
	if !exists {
		entry = indexEntry{
			ID:        p.ID,
			CreatedAt: p.CreatedAt,
			FileName:  filenameFor(p),
		}
	}
	entry.Name = p.Name
	entry.SourceCount = len(p.Sources)
	entry.UpdatedAt = p.UpdatedAt

	full := filepath.Join(r.dir, entry.FileName)
	if err := writeJSONAtomic(full, p); err != nil {
		return err
	}
	r.index[p.ID] = entry
	return r.saveIndexLocked()
}

// Delete removes the project file and its index entry. Missing id returns
// ErrNotFound; the on-disk file ends up clean either way.
func (r *JSONRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.index[id]
	if !ok {
		return ports.ErrNotFound
	}
	full := filepath.Join(r.dir, entry.FileName)
	if err := os.Remove(full); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("multitrack/jsonrepo: remove %s: %w", entry.FileName, err)
	}
	delete(r.index, id)
	return r.saveIndexLocked()
}

// --- internals -----------------------------------------------------------

func (r *JSONRepo) loadOrRebuildIndex() error {
	data, err := os.ReadFile(r.indexPath)
	if err == nil {
		var raw []indexEntry
		if err := json.Unmarshal(data, &raw); err == nil {
			for _, e := range raw {
				if e.ID == "" || e.FileName == "" {
					continue
				}
				r.index[e.ID] = e
			}
			return nil
		}
		// fall through to rebuild on parse error
	}
	return r.rebuildIndex()
}

func (r *JSONRepo) rebuildIndex() error {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return fmt.Errorf("multitrack/jsonrepo: readdir: %w", err)
	}
	r.index = make(map[string]indexEntry)
	for _, de := range entries {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".json") || de.Name() == "index.json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(r.dir, de.Name()))
		if err != nil {
			continue
		}
		var p domain.Project
		if err := json.Unmarshal(data, &p); err != nil || p.ID == "" {
			continue
		}
		// Defensive: skip files that aren't multitrack — guards against a
		// single-video file accidentally landing in this directory.
		if p.Kind != "" && p.Kind != domain.KindMultitrack {
			continue
		}
		r.index[p.ID] = indexEntry{
			ID:          p.ID,
			Name:        p.Name,
			SourceCount: len(p.Sources),
			CreatedAt:   p.CreatedAt,
			UpdatedAt:   p.UpdatedAt,
			FileName:   de.Name(),
		}
	}
	return r.saveIndexLocked()
}

func (r *JSONRepo) saveIndexLocked() error {
	rows := make([]indexEntry, 0, len(r.index))
	for _, e := range r.index {
		rows = append(rows, e)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].UpdatedAt.After(rows[j].UpdatedAt) })
	return writeJSONAtomic(r.indexPath, rows)
}

// filenameFor produces a stable, sortable filename.
func filenameFor(p *domain.Project) string {
	ts := p.CreatedAt.UTC().Format("2006-01-02_15-04-05")
	return fmt.Sprintf("%s_%s.json", ts, p.ID)
}

// writeJSONAtomic marshals v (pretty) and writes it via a temp file + rename
// so partial writes never corrupt the destination.
func writeJSONAtomic(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
