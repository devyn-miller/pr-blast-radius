// go-core/internal/mcpserver/server.go
package mcpserver

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/devyn-miller/pr-blast-radius/go-core/internal/astparse"
	"github.com/devyn-miller/pr-blast-radius/go-core/internal/cache"
	"github.com/devyn-miller/pr-blast-radius/go-core/internal/gitdiff"
	"github.com/devyn-miller/pr-blast-radius/go-core/internal/graph"
	"github.com/devyn-miller/pr-blast-radius/go-core/internal/severity"
)

type Server struct {
	extractor gitdiff.Extractor
	registry  *astparse.Registry
	store     cache.Store
	scorer    *severity.Scorer

	graphCache map[string]*graph.Graph
}

func NewServer(extractor gitdiff.Extractor, registry *astparse.Registry, store cache.Store, scorer *severity.Scorer) *Server {
	return &Server{
		extractor:  extractor,
		registry:   registry,
		store:      store,
		scorer:     scorer,
		graphCache: make(map[string]*graph.Graph),
	}
}

func (s *Server) Serve(ctx context.Context) error {
	mcpServer := server.NewMCPServer("blast-radius", "0.1.0")

	mcpServer.AddTool(
		mcp.NewTool("get_changed_symbols",
			mcp.WithDescription("Parses a git diff/range and maps changed line ranges to resolved AST symbols."),
			mcp.WithString("repo_path", mcp.Required()),
			mcp.WithString("ref_range", mcp.Required()),
		),
		s.toolGetChangedSymbols,
	)

	mcpServer.AddTool(
		mcp.NewTool("get_dependency_graph",
			mcp.WithDescription("Returns the full dependency graph for a repo, using the on-disk cache when valid."),
			mcp.WithString("repo_path", mcp.Required()),
			mcp.WithBoolean("force_rebuild"),
		),
		s.toolGetDependencyGraph,
	)

	mcpServer.AddTool(
		mcp.NewTool("compute_blast_radius",
			mcp.WithDescription("Performs reverse-reachability traversal and returns a ranked, severity-scored subgraph."),
			mcp.WithString("repo_path", mcp.Required()),
			mcp.WithString("ref_range", mcp.Required()),
			mcp.WithNumber("max_depth"),
		),
		s.toolComputeBlastRadius,
	)

	mcpServer.AddTool(
		mcp.NewTool("get_symbol_source",
			mcp.WithDescription("Returns literal source text for a symbol, for on-demand LLM context."),
			mcp.WithString("repo_path", mcp.Required()),
			mcp.WithString("symbol_id", mcp.Required()),
			mcp.WithNumber("max_bytes"),
		),
		s.toolGetSymbolSource,
	)

	return server.ServeStdio(mcpServer)
}

// --- Handlers: thin wiring only, all real logic lives in graph/severity/cache ---

func (s *Server) toolGetChangedSymbols(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoPath, err := req.RequireString("repo_path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	refRange, err := req.RequireString("ref_range")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	g, diffs, err := s.buildGraphAndDiff(ctx, repoPath, refRange)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	changed := graph.MapHunksToSymbols(g, diffs)
	return jsonToolResult(changed)
}

func (s *Server) toolGetDependencyGraph(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoPath, err := req.RequireString("repo_path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	forceRebuild := req.GetBool("force_rebuild", false)

	if forceRebuild {
		delete(s.graphCache, repoPath)
	}

	g, err := s.getOrBuildGraph(ctx, repoPath)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return jsonToolResult(g)
}

func (s *Server) toolComputeBlastRadius(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoPath, err := req.RequireString("repo_path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	refRange, err := req.RequireString("ref_range")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	maxDepth := req.GetInt("max_depth", 3)

	g, diffs, err := s.buildGraphAndDiff(ctx, repoPath, refRange)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	changed := graph.MapHunksToSymbols(g, diffs)
	if len(changed) == 0 {
		return mcp.NewToolResultError(fmt.Sprintf("no symbol-level changes detected for ref_range=%q", refRange)), nil
	}

	traverser := graph.NewTraverser(g)
	result := traverser.ComputeBlastRadius(changed, maxDepth)
	traverser.PopulateSeverity(&result, s.scorer)

	return jsonToolResult(result)
}

func (s *Server) toolGetSymbolSource(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoPath, err := req.RequireString("repo_path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	symbolID, err := req.RequireString("symbol_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	maxBytes := req.GetInt("max_bytes", 2000)

	g, err := s.getOrBuildGraph(ctx, repoPath)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	node, ok := g.Nodes[graph.NodeID(symbolID)]
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("unknown symbol_id %q", symbolID)), nil
	}

	src, truncated, err := readSymbolSource(repoPath, node, maxBytes)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return jsonToolResult(map[string]any{
		"symbol_id": symbolID, "source": src, "truncated": truncated,
	})
}

// --- shared helpers ---

func (s *Server) getOrBuildGraph(ctx context.Context, repoPath string) (*graph.Graph, error) {
	if g, ok := s.graphCache[repoPath]; ok {
		return g, nil
	}

	files, err := walkRepoSourceFiles(repoPath)
	if err != nil {
		return nil, err
	}

	var parser graph.FileParser = s.registry
	if s.store != nil {
		parser = cache.NewCachedRegistry(s.registry, s.store)
	}

	g, err := graph.BuildGraph(ctx, parser, files)
	if err != nil {
		return nil, err
	}
	g.Metadata.RepoPath = repoPath
	s.graphCache[repoPath] = g
	return g, nil
}

func (s *Server) buildGraphAndDiff(ctx context.Context, repoPath, refRange string) (*graph.Graph, []gitdiff.FileDiff, error) {
	g, err := s.getOrBuildGraph(ctx, repoPath)
	if err != nil {
		return nil, nil, err
	}

	rr := parseRefRange(refRange)
	diffs, _, err := s.extractor.ExtractDiff(ctx, repoPath, rr)
	if err != nil {
		return nil, nil, err
	}
	return g, diffs, nil
}

func parseRefRange(s string) gitdiff.RefRange {
	if s == "WORKING_TREE" {
		return gitdiff.RefRange{Kind: gitdiff.RefRangeWorkingTree}
	}
	if containsRangeMarker(s) {
		parts := splitRangeMarker(s)
		return gitdiff.RefRange{Kind: gitdiff.RefRangeCommitRange, FromRef: parts[0], ToRef: parts[1]}
	}
	return gitdiff.RefRange{Kind: gitdiff.RefRangeSingleCommit, ToRef: s}
}