// Command loom is a CLI for managing prompts (list, get, store, promote, delete, tag).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/klejdi94/loom/core"
	"github.com/klejdi94/loom/registry"
)

func main() {
	regDir := flag.String("registry", ".loom", "Registry directory (file backend)")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}
	reg, err := registry.NewFileRegistry(*regDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "registry:", err)
		os.Exit(1)
	}
	ctx := context.Background()
	cmd := args[0]
	rest := args[1:]
	switch cmd {
	case "list":
		list(ctx, reg, rest)
	case "get":
		get(ctx, reg, rest)
	case "store":
		store(ctx, reg, rest)
	case "promote":
		promote(ctx, reg, rest)
	case "delete":
		deleteCmd(ctx, reg, rest)
	case "tag":
		tag(ctx, reg, rest)
	case "versions":
		versions(ctx, reg, rest)
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: loom [ -registry <dir> ] <command> [args]

Commands:
  list                    List all prompts
  get <id> [version]      Get prompt (default: production version)
  store                   Store prompt from stdin (JSON)
  promote <id> <version> [stage]  Promote version (stage: dev|staging|production)
  delete <id> <version>  Delete a version
  tag <id> <version> <tag...>  Add tags
  versions <id>          List versions for an id

Registry: file-based in -registry directory (default: .loom)
`)
}

func list(ctx context.Context, reg registry.Registry, args []string) {
	filter := registry.Filter{Limit: 500}
	prompts, err := reg.List(ctx, filter)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, p := range prompts {
		fmt.Printf("%s\t%s\t%s\n", p.ID, p.Version, p.Name)
	}
}

func get(ctx context.Context, reg registry.Registry, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "get requires <id> [version]")
		os.Exit(1)
	}
	id := args[0]
	version := ""
	if len(args) >= 2 {
		version = args[1]
	}
	var p *core.Prompt
	var err error
	if version == "" {
		p, err = reg.GetProduction(ctx, id)
	} else {
		p, err = reg.Get(ctx, id, version)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(p); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func store(ctx context.Context, reg registry.Registry, args []string) {
	var p core.Prompt
	if err := json.NewDecoder(os.Stdin).Decode(&p); err != nil {
		fmt.Fprintln(os.Stderr, "decode:", err)
		os.Exit(1)
	}
	if p.ID == "" || p.Version == "" {
		fmt.Fprintln(os.Stderr, "prompt must have id and version")
		os.Exit(1)
	}
	if err := reg.Store(ctx, &p); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("stored %s@%s\n", p.ID, p.Version)
}

func promote(ctx context.Context, reg registry.Registry, args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "promote requires <id> <version> [stage]")
		os.Exit(1)
	}
	id, version := args[0], args[1]
	stage := registry.StageProduction
	if len(args) >= 3 {
		switch strings.ToLower(args[2]) {
		case "dev":
			stage = registry.StageDev
		case "staging":
			stage = registry.StageStaging
		case "production":
			stage = registry.StageProduction
		default:
			fmt.Fprintln(os.Stderr, "stage must be dev|staging|production")
			os.Exit(1)
		}
	}
	if err := reg.Promote(ctx, id, version, stage); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("promoted %s@%s to %s\n", id, version, stage)
}

func deleteCmd(ctx context.Context, reg registry.Registry, args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "delete requires <id> <version>")
		os.Exit(1)
	}
	if err := reg.Delete(ctx, args[0], args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("deleted %s@%s\n", args[0], args[1])
}

func tag(ctx context.Context, reg registry.Registry, args []string) {
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "tag requires <id> <version> <tag...>")
		os.Exit(1)
	}
	id, version := args[0], args[1]
	tags := args[2:]
	if err := reg.Tag(ctx, id, version, tags); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("tagged %s@%s with %v\n", id, version, tags)
}

func versions(ctx context.Context, reg registry.Registry, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "versions requires <id>")
		os.Exit(1)
	}
	infos, err := reg.ListVersions(ctx, args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, vi := range infos {
		fmt.Printf("%s\t%s\t%v\n", vi.Version, vi.Stage, vi.Tags)
	}
}
