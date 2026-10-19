// go-core/internal/mcpserver/helpers.go
package mcpserver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/devyn-miller/pr-blast-radius/go-core/internal/graph"
)

// jsonToolResult marshals any value into an MCP tool result. Every handler
// funnels through this ONE function so the "MCP responses are JSON-encoded
// text content" convention (which the Python mcp_client._parse relies on)
// is enforced in exactly one place, not re-implemented per handler.
func jsonToolResult(v any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling response: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// walkRepoSourceFiles collects every .go/.py/.ts/.tsx file under repoPath,
// skipping common noise directories. This deliberately does NOT filter
// by "changed files only" — BuildGraph needs the FULL repo graph (a
// changed function's caller might live in a completely untouched file),
// per the architecture note in graph.go's FileSource comment.
func walkRepoSourceFiles(repoPath string) ([]graph.FileSource, error) {
	var files []graph.FileSource
	skipDirs := map[string]bool{
		".git": true, "node_modules": true, "__pycache__": true,
		".venv": true, "venv": true, "dist": true, "build": true,
	}
	supportedExt := map[string]bool{".go": true, ".py": true, ".ts": true, ".tsx": true}

	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !supportedExt[filepath.Ext(path)] {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("mcpserver: reading %s: %w", path, err)
		}
		relPath, err := filepath.Rel(repoPath, path)
		if err != nil {
			relPath = path
		}
		files = append(files, graph.FileSource{Path: relPath, Source: src})
		return nil
	})
	return files, err
}

// readSymbolSource opens the node's file and returns its byte range,
// bounded by maxBytes — the bound is enforced HERE, not left to the
// Python side, since it's the Go server's responsibility to never hand
// back an unbounded blob regardless of what the caller requests.
func readSymbolSource(repoPath string, node graph.Node, maxBytes int) (source string, truncated bool, err error) {
	fullPath := filepath.Join(repoPath, node.FilePath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", false, fmt.Errorf("mcpserver: reading source file %s: %w", fullPath, err)
	}

	lines := strings.Split(string(data), "\n")
	if node.StartLine < 1 || node.StartLine > len(lines) {
		return "", false, fmt.Errorf("mcpserver: symbol %s has invalid StartLine %d for file with %d lines",
			node.ID, node.StartLine, len(lines))
	}
	endLine := node.EndLine
	if endLine > len(lines) {
		endLine = len(lines)
	}

	snippet := strings.Join(lines[node.StartLine-1:endLine], "\n")
	if len(snippet) > maxBytes {
		return snippet[:maxBytes], true, nil
	}
	return snippet, false, nil
}

// rangeMarkers is checked in order: "..." must come first because it
// contains "..". Both map to the same tree-to-tree diff (the extractor does
// not compute a merge base), which is exactly git's `diff A..B` semantics.
var rangeMarkers = []string{"...", ".."}

func containsRangeMarker(s string) bool {
	return strings.Contains(s, "..")
}

func splitRangeMarker(s string) [2]string {
	for _, marker := range rangeMarkers {
		if from, to, ok := strings.Cut(s, marker); ok {
			// Like git, an omitted side means HEAD ("..feature", "main..").
			if from == "" {
				from = "HEAD"
			}
			if to == "" {
				to = "HEAD"
			}
			return [2]string{from, to}
		}
	}
	return [2]string{s, "HEAD"}
}
