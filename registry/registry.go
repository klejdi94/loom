// Package registry provides prompt versioning and storage backends.
package registry

import (
	"context"
	"time"

	"github.com/klejdi94/loom/core"
)

// Stage represents a deployment stage (e.g. dev, staging, production).
type Stage string

const (
	StageDev        Stage = "dev"
	StageStaging    Stage = "staging"
	StageProduction Stage = "production"
)

// VersionInfo holds metadata about a stored prompt version.
type VersionInfo struct {
	ID        string
	Version   string
	Stage     Stage
	Tags      []string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Filter limits which prompts are returned by List.
type Filter struct {
	IDs    []string
	Stage  Stage
	Tags   []string
	Limit  int
	Offset int
}

// Registry stores and retrieves versioned prompts.
type Registry interface {
	Store(ctx context.Context, prompt *core.Prompt) error
	Get(ctx context.Context, id, version string) (*core.Prompt, error)
	GetProduction(ctx context.Context, id string) (*core.Prompt, error)
	List(ctx context.Context, filter Filter) ([]*core.Prompt, error)
	ListVersions(ctx context.Context, id string) ([]VersionInfo, error)
	Promote(ctx context.Context, id, version string, stage Stage) error
	Delete(ctx context.Context, id, version string) error
	Tag(ctx context.Context, id, version string, tags []string) error
}
