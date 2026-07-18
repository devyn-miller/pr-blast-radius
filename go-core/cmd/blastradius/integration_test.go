package main

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devyn-miller/pr-blast-radius/go-core/internal/astparse"
	"github.com/devyn-miller/pr-blast-radius/go-core/internal/cache"
	"github.com/devyn-miller/pr-blast-radius/go-core/internal/gitdiff"
	"github.com/devyn-miller/pr-blast-radius/go-core/internal/graph"
	"github.com/devyn-miller/pr-blast-radius/go-core/internal/severity"
)

// TestFullPipeline_TinyShop_GoldenFile runs the complete blast radius analysis
// against the TinyShop fixture repository with a WORKING_TREE diff.
//
// Test scenario:
// 1. The TinyShop repo has been committed with the initial state
// 2. We make an uncommitted change to discount_engine.py: add a `currency` parameter
//    to the apply_discount() signature
// 3. We run the full pipeline (parse → resolve → graph → traverse → severity)
// 4. We verify that OrderService.price appears in the blast radius (it calls apply_discount)
// 5. We verify that InventoryService.reserve does NOT appear (independent of pricing)
func TestFullPipeline_TinyShop_GoldenFile(t *testing.T) {
	// Find the TinyShop fixture relative to this test file
	tinyshopPath := filepath.Join("..", "..", "..", "examples", "tinyshop")
	absPath, err := filepath.Abs(tinyshopPath)
	if err != nil {
		t.Fatalf("Failed to resolve tinyshop path: %v", err)
	}

	// Verify it's a git repo
	if _, err := os.Stat(filepath.Join(absPath, ".git")); os.IsNotExist(err) {
		t.Fatalf("TinyShop is not a git repo - run 'git init' in %s", absPath)
	}

	// Path to the file we'll modify
	discountEnginePath := filepath.Join(absPath, "services", "pricing", "discount_engine.py")

	// Read original content
	originalContent, err := os.ReadFile(discountEnginePath)
	if err != nil {
		t.Fatalf("Failed to read discount_engine.py: %v", err)
	}

	// Make an uncommitted change: add a `currency` parameter to apply_discount signature
	// Original: def apply_discount(self, base_price: Money, customer_tier: str) -> Money:
	// Modified: def apply_discount(self, base_price: Money, customer_tier: str, currency: str = "USD") -> Money:
	modifiedContent := strings.Replace(
		string(originalContent),
		"def apply_discount(self, base_price: Money, customer_tier: str) -> Money:",
		"def apply_discount(self, base_price: Money, customer_tier: str, currency: str = \"USD\") -> Money:",
		1,
	)

	// Write the modified content (uncommitted change)
	if err := os.WriteFile(discountEnginePath, []byte(modifiedContent), 0644); err != nil {
		t.Fatalf("Failed to write modified discount_engine.py: %v", err)
	}

	// Ensure we restore the original content after the test
	defer func() {
		if err := os.WriteFile(discountEnginePath, originalContent, 0644); err != nil {
			t.Errorf("Failed to restore discount_engine.py: %v", err)
		}
	}()

	// Now run the full pipeline
	ctx := context.Background()

	// Step 1: Extract WORKING_TREE changes (uncommitted diff)
	extractor := gitdiff.NewGoGitExtractor()
	refRange := gitdiff.RefRange{Kind: gitdiff.RefRangeWorkingTree}
	changes, _, err := extractor.ExtractDiff(ctx, absPath, refRange)
	if err != nil {
		t.Fatalf("ExtractDiff failed: %v", err)
	}

	if len(changes) == 0 {
		t.Fatal("Expected uncommitted changes in discount_engine.py, but got none")
	}

	// Verify we detected the change to discount_engine.py
	foundDiscountChange := false
	for _, change := range changes {
		if strings.HasSuffix(change.FilePath, "discount_engine.py") {
			foundDiscountChange = true
			if len(change.Hunks) == 0 {
				t.Error("discount_engine.py changed but has no hunks")
			}
			t.Logf("Found change in discount_engine.py with %d hunks", len(change.Hunks))
		}
	}
	if !foundDiscountChange {
		t.Fatal("Expected to find uncommitted change to discount_engine.py")
	}

	// Step 2: Set up parsers and cache
	goParser, err := astparse.NewGoParser()
	if err != nil {
		t.Fatalf("Failed to create Go parser: %v", err)
	}
	pyParser, err := astparse.NewPythonParser()
	if err != nil {
		t.Fatalf("Failed to create Python parser: %v", err)
	}
	registry := astparse.NewRegistry(goParser, pyParser)

	// Use in-memory SQLite for caching
	store, err := cache.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create cache store: %v", err)
	}
	defer store.Close()

	cachedReg := cache.NewCachedRegistry(registry, store)

	// Step 3: Collect all source files in the tinyshop repo
	var fileSources []graph.FileSource

	err = filepath.WalkDir(absPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "__pycache__") {
			return filepath.SkipDir
		}
		if !d.IsDir() && (strings.HasSuffix(path, ".py") || strings.HasSuffix(path, ".go")) {
			source, err := os.ReadFile(path)
			if err != nil {
				t.Logf("Warning: failed to read %s: %v", path, err)
				return nil
			}

			// Use relative path from repo root to match diff output
			relPath, err := filepath.Rel(absPath, path)
			if err != nil {
				t.Logf("Warning: failed to compute relative path for %s: %v", path, err)
				relPath = path
			}

			fileSources = append(fileSources, graph.FileSource{
				Path:   relPath,
				Source: source,
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to walk tinyshop directory: %v", err)
	}

	// Step 4: Build the full project graph
	depGraph, err := graph.BuildGraph(ctx, cachedReg, fileSources)
	if err != nil {
		t.Fatalf("Failed to build graph: %v", err)
	}

	t.Logf("Built graph with %d nodes", len(depGraph.Nodes))

	// Step 5: Map hunks to changed symbols
	changedSymbols := graph.MapHunksToSymbols(depGraph, changes)
	if len(changedSymbols) == 0 {
		t.Fatal("Expected at least one changed symbol (apply_discount), got none")
	}

	t.Logf("Found %d changed symbol(s) in discount_engine.py", len(changedSymbols))

	// Step 6: Compute blast radius
	traverser := graph.NewTraverser(depGraph)
	blastRadius := traverser.ComputeBlastRadius(changedSymbols, 5)

	// Step 7: Populate severity scores
	scorer := severity.NewScorer(severity.DefaultWeights)
	traverser.PopulateSeverity(&blastRadius, scorer)

	// Step 8: Verify golden assertions
	// We expect OrderService.price to be affected (it calls apply_discount)
	// We expect InventoryService.reserve to NOT be affected (independent)
	foundOrderServicePrice := false
	foundInventoryServiceReserve := false

	for _, affected := range blastRadius.AffectedNodes {
		node := depGraph.Nodes[affected.NodeID]
		qualName := node.QualifiedName

		t.Logf("Affected: %s (distance=%d, severity=%.3f)", qualName, affected.Distance, affected.SeverityScore)

		if strings.Contains(qualName, "OrderService") && strings.Contains(qualName, "price") {
			foundOrderServicePrice = true
		}
		if strings.Contains(qualName, "InventoryService") && strings.Contains(qualName, "reserve") {
			foundInventoryServiceReserve = true
		}
	}

	// GOLDEN ASSERTIONS
	if !foundOrderServicePrice {
		t.Error("Expected OrderService.price in blast radius (it calls apply_discount), but it was not found")
	}
	if foundInventoryServiceReserve {
		t.Error("InventoryService.reserve should NOT be in blast radius (independent of pricing), but it was found")
	}

	// Verify we got a non-empty blast radius
	if blastRadius.TotalNodesAffected == 0 {
		t.Error("Expected non-zero TotalNodesAffected")
	}

	t.Logf("✅ Integration test passed: %d total nodes affected, OrderService.price found, InventoryService.reserve not found",
		blastRadius.TotalNodesAffected)
}
