# Implementing storage backends

The registry interface is in `github.com/klejdi94/loom/registry`:

```go
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
```

## Provided implementations

- **MemoryRegistry**: In-memory map; good for tests and single-process.
- **FileRegistry**: JSON files under a directory; one file per `id_version.json`, plus `_meta.json` for stage/tags.
- **PostgresRegistry**: Single table with JSONB for variables, examples, metadata, tags; requires `*sql.DB` with a PostgreSQL driver (e.g. `github.com/lib/pq`).

## Implementing a new backend

1. **Serialization**: `core.Prompt` can be JSON-encoded; `Variable.Validation` is a function and will be omitted when decoding, so loaded prompts will have `Validation == nil` for variables.
2. **Stages and production**: Maintain a notion of “production” per id (e.g. a row or key with `stage = 'production'`, or a separate `production` map from id → version).
3. **Copy on read**: Return `prompt.Copy()` (or equivalent) from `Get`/`GetProduction`/`List` so callers cannot mutate stored data.
4. **Concurrency**: Document whether the implementation is safe for concurrent use; FileRegistry and PostgresRegistry use locks or the DB’s transactional semantics.

## Using the CLI with a file registry

The CLI uses the file backend by default with `-registry .loom`. Point it at a directory that will hold the JSON files and meta:

```bash
loom -registry /path/to/prompts list
```

For PostgreSQL you would use the library in code; the CLI does not currently accept a DSN.
