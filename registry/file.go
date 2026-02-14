// Package registry file-based storage implementation.
package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/klejdi94/loom/core"
)

// FileRegistry stores prompts as JSON files in a directory.
// File names: {id}_{version}.json (sanitized). Stage and tags in a separate meta file or embedded in filename is not used; stage/tags kept in memory for compatibility with interface.
type FileRegistry struct {
	dir    string
	mu     sync.RWMutex
	stages map[string]string            // id -> version for production
	tags   map[string][]string         // id:version -> tags
	meta   map[string]map[string]stageMeta // id -> version -> meta
}

type stageMeta struct {
	Stage Stage   `json:"stage"`
	Tags  []string `json:"tags,omitempty"`
}

// NewFileRegistry creates a file-based registry rooted at dir.
func NewFileRegistry(dir string) (*FileRegistry, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("file registry: %w", err)
	}
	r := &FileRegistry{
		dir:    dir,
		stages: make(map[string]string),
		tags:   make(map[string][]string),
		meta:   make(map[string]map[string]stageMeta),
	}
	if err := r.loadMeta(); err != nil {
		return nil, err
	}
	return r, nil
}

func (f *FileRegistry) filename(id, version string) string {
	safeID := strings.ReplaceAll(strings.ReplaceAll(id, string(filepath.Separator), "_"), ":", "_")
	safeVer := strings.ReplaceAll(strings.ReplaceAll(version, string(filepath.Separator), "_"), ":", "_")
	return filepath.Join(f.dir, safeID+"_"+safeVer+".json")
}

func (f *FileRegistry) metaPath() string {
	return filepath.Join(f.dir, "_meta.json")
}

func (f *FileRegistry) loadMeta() error {
	path := f.metaPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var out struct {
		Production map[string]string                `json:"production"`
		Meta       map[string]map[string]stageMeta `json:"meta"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	if out.Production != nil {
		f.stages = out.Production
	}
	if out.Meta != nil {
		f.meta = out.Meta
		for id, vers := range f.meta {
			for v, m := range vers {
				f.tags[f.key(id, v)] = append([]string(nil), m.Tags...)
			}
		}
	}
	return nil
}

func (f *FileRegistry) key(id, version string) string {
	return id + ":" + version
}

func (f *FileRegistry) saveMeta() error {
	path := f.metaPath()
	out := struct {
		Production map[string]string                `json:"production"`
		Meta       map[string]map[string]stageMeta `json:"meta"`
	}{
		Production: f.stages,
		Meta:       f.meta,
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Store saves a prompt as a JSON file.
func (f *FileRegistry) Store(ctx context.Context, prompt *core.Prompt) error {
	if prompt == nil || prompt.ID == "" || prompt.Version == "" {
		return fmt.Errorf("file registry: prompt id and version required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	path := f.filename(prompt.ID, prompt.Version)
	// Marshal prompt; Validation funcs will be omitted
	payload, err := json.MarshalIndent(prompt, "", "  ")
	if err != nil {
		return fmt.Errorf("file registry encode: %w", err)
	}
	if err := os.WriteFile(path, payload, 0644); err != nil {
		return err
	}
	if f.meta[prompt.ID] == nil {
		f.meta[prompt.ID] = make(map[string]stageMeta)
	}
	if _, ok := f.meta[prompt.ID][prompt.Version]; !ok {
		f.meta[prompt.ID][prompt.Version] = stageMeta{Stage: StageDev}
	}
	return f.saveMeta()
}

// Get reads a prompt from disk.
func (f *FileRegistry) Get(ctx context.Context, id, version string) (*core.Prompt, error) {
	f.mu.RLock()
	path := f.filename(id, version)
	f.mu.RUnlock()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, core.ErrPromptNotFound
		}
		return nil, err
	}
	var p core.Prompt
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("file registry decode: %w", err)
	}
	if p.ID != id || p.Version != version {
		p.ID = id
		p.Version = version
	}
	return p.Copy(), nil
}

// GetProduction returns the promoted production version for id.
func (f *FileRegistry) GetProduction(ctx context.Context, id string) (*core.Prompt, error) {
	f.mu.RLock()
	version, ok := f.stages[id]
	f.mu.RUnlock()
	if !ok || version == "" {
		return nil, core.ErrPromptNotFound
	}
	return f.Get(ctx, id, version)
}

// List lists prompts matching the filter (scans directory).
func (f *FileRegistry) List(ctx context.Context, filter Filter) ([]*core.Prompt, error) {
	f.mu.RLock()
	entries, err := os.ReadDir(f.dir)
	f.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	var out []*core.Prompt
	offset := filter.Offset
	limit := filter.Limit
	if limit <= 0 {
		limit = 1000
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || e.Name() == "_meta.json" {
			continue
		}
		path := filepath.Join(f.dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var p core.Prompt
		if err := json.Unmarshal(data, &p); err != nil {
			continue
		}
		if len(filter.IDs) > 0 && !contains(filter.IDs, p.ID) {
			continue
		}
		f.mu.RLock()
		st := f.meta[p.ID][p.Version].Stage
		tags := f.tags[f.key(p.ID, p.Version)]
		f.mu.RUnlock()
		if filter.Stage != "" && st != filter.Stage {
			continue
		}
		if len(filter.Tags) > 0 && !hasAll(tags, filter.Tags) {
			continue
		}
		if offset > 0 {
			offset--
			continue
		}
		out = append(out, p.Copy())
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// ListVersions returns version info for an id (from meta and existing files).
func (f *FileRegistry) ListVersions(ctx context.Context, id string) ([]VersionInfo, error) {
	f.mu.RLock()
	versMeta := f.meta[id]
	f.mu.RUnlock()
	if versMeta == nil {
		return nil, nil
	}
	var infos []VersionInfo
	for version := range versMeta {
		p, err := f.Get(ctx, id, version)
		if err != nil {
			continue
		}
		f.mu.RLock()
		st := versMeta[version].Stage
		tags := f.tags[f.key(id, version)]
		f.mu.RUnlock()
		infos = append(infos, VersionInfo{
			ID:        id,
			Version:   version,
			Stage:     st,
			Tags:      append([]string(nil), tags...),
			CreatedAt: p.CreatedAt,
			UpdatedAt: p.UpdatedAt,
		})
	}
	return infos, nil
}

// Promote sets the stage for id+version and updates production pointer.
func (f *FileRegistry) Promote(ctx context.Context, id, version string, stage Stage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := f.filename(id, version)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return core.ErrPromptNotFound
		}
		return err
	}
	if f.meta[id] == nil {
		f.meta[id] = make(map[string]stageMeta)
	}
	f.meta[id][version] = stageMeta{Stage: stage, Tags: f.tags[f.key(id, version)]}
	if stage == StageProduction {
		f.stages[id] = version
	}
	return f.saveMeta()
}

// Delete removes the prompt file and meta.
func (f *FileRegistry) Delete(ctx context.Context, id, version string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := f.filename(id, version)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if f.stages[id] == version {
		delete(f.stages, id)
	}
	if f.meta[id] != nil {
		delete(f.meta[id], version)
	}
	delete(f.tags, f.key(id, version))
	return f.saveMeta()
}

// Tag sets tags for a prompt version.
func (f *FileRegistry) Tag(ctx context.Context, id, version string, tags []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.meta[id] == nil || f.meta[id][version].Stage == "" {
		path := f.filename(id, version)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return core.ErrPromptNotFound
			}
			return err
		}
		if f.meta[id] == nil {
			f.meta[id] = make(map[string]stageMeta)
		}
		f.meta[id][version] = stageMeta{Stage: StageDev, Tags: tags}
	} else {
		m := f.meta[id][version]
		m.Tags = append([]string(nil), tags...)
		f.meta[id][version] = m
	}
	f.tags[f.key(id, version)] = append([]string(nil), tags...)
	return f.saveMeta()
}
