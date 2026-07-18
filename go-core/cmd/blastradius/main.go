package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/devyn-miller/pr-blast-radius/go-core/internal/astparse"
	"github.com/devyn-miller/pr-blast-radius/go-core/internal/cache"
	"github.com/devyn-miller/pr-blast-radius/go-core/internal/gitdiff"
	"github.com/devyn-miller/pr-blast-radius/go-core/internal/mcpserver"
	"github.com/devyn-miller/pr-blast-radius/go-core/internal/severity"
)

func main() {
	rootCmd := &cobra.Command{Use: "blastradius"}

	rootCmd.AddCommand(&cobra.Command{
		Use:   "mcp-serve",
		Short: "Run as an MCP server over stdio",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPServe()
		},
	})

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runMCPServe() error {
	goParser, err := astparse.NewGoParser()
	if err != nil {
		return err
	}
	pyParser, err := astparse.NewPythonParser()
	if err != nil {
		return err
	}
	registry := astparse.NewRegistry(goParser, pyParser)

	cacheDB := os.Getenv("BLASTRADIUS_CACHE_DB")
	if cacheDB == "" {
		cacheDB = "./.blastradius-cache.db"
	}
	store, err := cache.NewSQLiteStore(cacheDB)
	if err != nil {
		return err
	}
	defer store.Close()

	extractor := gitdiff.NewGoGitExtractor()
	scorer := severity.NewScorer(severity.DefaultWeights)

	srv := mcpserver.NewServer(extractor, registry, store, scorer)
	return srv.Serve(context.Background())
}