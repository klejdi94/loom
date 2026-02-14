package registry

import (
	"context"
	"fmt"
	"sync"

	"github.com/klejdi94/loom/core"
)

// MemoryRegistry is an in-memory registry for prompts (testing and single-process use).
type MemoryRegistry struct {
	mu        sync.RWMutex
	prompts   map[string]map[string]*core.Prompt // id -> version -> prompt
	production map[string]string                 // id -> version
	stages    map[string]map[string]Stage         // id -> version -> stage
	tags      map[string][]string // id:version -> tags
}

// NewMemoryRegistry creates an empty in-memory registry.
func NewMemoryRegistry() *MemoryRegistry {
	return &MemoryRegistry{
		prompts:    make(map[string]map[string]*core.Prompt),
		production: make(map[string]string),
		stages:     make(map[string]map[string]Stage),
		tags:       make(map[string][]string),
	}
}

func (m *MemoryRegistry) key(id, version string) string {
	return id + ":" + version
}

// Store saves a prompt. Overwrites if id+version already exists.
func (m *MemoryRegistry) Store(ctx context.Context, prompt *core.Prompt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if prompt == nil {
		return fmt.Errorf("prompt is nil")
	}
	if prompt.ID == "" || prompt.Version == "" {
		return fmt.Errorf("prompt id and version are required")
	}
	if m.prompts[prompt.ID] == nil {
		m.prompts[prompt.ID] = make(map[string]*core.Prompt)
	}
	// Copy so caller cannot mutate stored prompt
	p := copyPrompt(prompt)
	m.prompts[prompt.ID][prompt.Version] = p
	if m.stages[prompt.ID] == nil {
		m.stages[prompt.ID] = make(map[string]Stage)
	}
	if _, ok := m.stages[prompt.ID][prompt.Version]; !ok {
		m.stages[prompt.ID][prompt.Version] = StageDev
	}
	return nil
}

// Get returns a prompt by id and version.
func (m *MemoryRegistry) Get(ctx context.Context, id, version string) (*core.Prompt, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	versions, ok := m.prompts[id]
	if !ok {
		return nil, core.ErrPromptNotFound
	}
	p, ok := versions[version]
	if !ok {
		return nil, core.ErrPromptNotFound
	}
	return copyPrompt(p), nil
}

// GetProduction returns the prompt currently promoted to production for the id.
func (m *MemoryRegistry) GetProduction(ctx context.Context, id string) (*core.Prompt, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	version, ok := m.production[id]
	if !ok {
		return nil, core.ErrPromptNotFound
	}
	versions, ok := m.prompts[id]
	if !ok {
		return nil, core.ErrPromptNotFound
	}
	p, ok := versions[version]
	if !ok {
		return nil, core.ErrPromptNotFound
	}
	return copyPrompt(p), nil
}

// List returns prompts matching the filter.
func (m *MemoryRegistry) List(ctx context.Context, filter Filter) ([]*core.Prompt, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*core.Prompt
	offset := filter.Offset
	limit := filter.Limit
	if limit <= 0 {
		limit = 1000
	}
	for id, versions := range m.prompts {
		if len(filter.IDs) > 0 && !contains(filter.IDs, id) {
			continue
		}
		for _, p := range versions {
			if filter.Stage != "" {
				st := m.stages[id]
				if st == nil || st[p.Version] != filter.Stage {
					continue
				}
			}
			if len(filter.Tags) > 0 {
				k := m.key(id, p.Version)
				if !hasAll(m.tags[k], filter.Tags) {
					continue
				}
			}
			if offset > 0 {
				offset--
				continue
			}
			out = append(out, copyPrompt(p))
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}

// ListVersions returns version info for an id.
func (m *MemoryRegistry) ListVersions(ctx context.Context, id string) ([]VersionInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	versions, ok := m.prompts[id]
	if !ok {
		return nil, nil
	}
	var infos []VersionInfo
	for v, p := range versions {
		st := StageDev
		if s, ok := m.stages[id]; ok {
			st = s[v]
		}
		tags := m.tags[m.key(id, v)]
		infos = append(infos, VersionInfo{
			ID:        id,
			Version:   v,
			Stage:     st,
			Tags:      append([]string(nil), tags...),
			CreatedAt: p.CreatedAt,
			UpdatedAt: p.UpdatedAt,
		})
	}
	return infos, nil
}

// Promote sets the stage for a given id+version.
func (m *MemoryRegistry) Promote(ctx context.Context, id, version string, stage Stage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	versions, ok := m.prompts[id]
	if !ok {
		return core.ErrPromptNotFound
	}
	if _, ok := versions[version]; !ok {
		return core.ErrPromptNotFound
	}
	if m.stages[id] == nil {
		m.stages[id] = make(map[string]Stage)
	}
	m.stages[id][version] = stage
	if stage == StageProduction {
		m.production[id] = version
	}
	return nil
}

// Delete removes a prompt version.
func (m *MemoryRegistry) Delete(ctx context.Context, id, version string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	versions, ok := m.prompts[id]
	if !ok {
		return core.ErrPromptNotFound
	}
	if _, ok := versions[version]; !ok {
		return core.ErrPromptNotFound
	}
	delete(versions, version)
	if m.production[id] == version {
		delete(m.production, id)
	}
	if m.stages[id] != nil {
		delete(m.stages[id], version)
	}
	delete(m.tags, m.key(id, version))
	return nil
}

// Tag sets tags for a prompt version.
func (m *MemoryRegistry) Tag(ctx context.Context, id, version string, tags []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	versions, ok := m.prompts[id]
	if !ok {
		return core.ErrPromptNotFound
	}
	if _, ok := versions[version]; !ok {
		return core.ErrPromptNotFound
	}
	m.tags[m.key(id, version)] = append([]string(nil), tags...)
	return nil
}

func copyPrompt(p *core.Prompt) *core.Prompt {
	return p.Copy()
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func hasAll(have, need []string) bool {
	for _, n := range need {
		found := false
		for _, h := range have {
			if h == n {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
