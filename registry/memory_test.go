package registry

import (
	"context"
	"testing"

	"github.com/klejdi94/loom/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryRegistry_StoreGet(t *testing.T) {
	ctx := context.Background()
	reg := NewMemoryRegistry()
	p := &core.Prompt{ID: "p1", Version: "1.0.0", Template: "hello"}
	err := reg.Store(ctx, p)
	require.NoError(t, err)
	got, err := reg.Get(ctx, "p1", "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, "p1", got.ID)
	assert.Equal(t, "1.0.0", got.Version)
	assert.Equal(t, "hello", got.Template)
}

func TestMemoryRegistry_GetNotFound(t *testing.T) {
	ctx := context.Background()
	reg := NewMemoryRegistry()
	_, err := reg.Get(ctx, "missing", "1.0.0")
	assert.ErrorIs(t, err, core.ErrPromptNotFound)
}

func TestMemoryRegistry_PromoteGetProduction(t *testing.T) {
	ctx := context.Background()
	reg := NewMemoryRegistry()
	p := &core.Prompt{ID: "p1", Version: "1.0.0"}
	require.NoError(t, reg.Store(ctx, p))
	require.NoError(t, reg.Promote(ctx, "p1", "1.0.0", StageProduction))
	prod, err := reg.GetProduction(ctx, "p1")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", prod.Version)
}

func TestMemoryRegistry_ListVersions(t *testing.T) {
	ctx := context.Background()
	reg := NewMemoryRegistry()
	reg.Store(ctx, &core.Prompt{ID: "p1", Version: "1.0.0"})
	reg.Store(ctx, &core.Prompt{ID: "p1", Version: "2.0.0"})
	vers, err := reg.ListVersions(ctx, "p1")
	require.NoError(t, err)
	assert.Len(t, vers, 2)
}
