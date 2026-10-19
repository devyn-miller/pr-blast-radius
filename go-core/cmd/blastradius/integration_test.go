package main

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/devyn-miller/pr-blast-radius/go-core/internal/astparse"
	"github.com/devyn-miller/pr-blast-radius/go-core/internal/cache"
	"github.com/devyn-miller/pr-blast-radius/go-core/internal/gitdiff"
	"github.com/devyn-miller/pr-blast-radius/go-core/internal/graph"
	"github.com/devyn-miller/pr-blast-radius/go-core/internal/severity"
)

// TestFullPipeline_TinyShop_GoldenFile runs the complete blast radius analysis
// against the TinyShop fixture with a WORKING_TREE diff that adds a
// `currency` parameter to DiscountEngine.apply_discount.
//
// TinyShop's call chain is Python-only. Four route handlers reach pricing:
//
//	POST /orders    create_order -> orders.price() -> self.pricing.apply_discount()
//	POST /quote     quote_order  -> orders.price() -> ...
//	POST /cart      price_cart   -> orders.price() -> ...
//	POST /checkout  checkout     -> orders.price() -> ...
//	GET /inventory/<sku> reserve_stock -> inventory.reserve()   (independent)
//
// That makes OrderService.price the graph's fan-in hotspot (in-degree 4, the
// graph maximum), which is the point of the fixture: severity has to separate
// the one shared chokepoint from the four thin handlers above it.
//
// api/routes.py also constructs DiscountEngine() at module level, and the
// diff marks the DiscountEngine class Modified, so that module node is
// affected at distance 1 as well.
func TestFullPipeline_TinyShop_GoldenFile(t *testing.T) {
	depGraph, blastRadius := runTinyShopPipeline(t)

	type hit struct {
		distance int
		node     graph.Node
	}
	affected := map[string]hit{}
	for _, an := range blastRadius.AffectedNodes {
		node := depGraph.Nodes[an.NodeID]
		key := node.QualifiedName
		if node.Kind == astparse.KindModule {
			key = node.FilePath + ":" + node.QualifiedName
		}
		affected[key] = hit{an.Distance, node}
		t.Logf("Affected: %s (distance=%d, severity=%.3f, reasons=%v)", key, an.Distance, an.SeverityScore, an.SeverityReasons)
	}

	want := map[string]int{
		"OrderService.price":     1,
		"api/routes.py:<module>": 1,
		"create_order":           2,
		"quote_order":            2,
		"price_cart":             2,
		"checkout":               2,
	}
	got := map[string]int{}
	for k, h := range affected {
		got[k] = h.distance
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("affected nodes (name -> distance):\n got  %v\n want %v", got, want)
	}

	for handler, tag := range map[string]string{
		"create_order": "route:POST:/orders",
		"quote_order":  "route:POST:/quote",
		"price_cart":   "route:POST:/cart",
		"checkout":     "route:POST:/checkout",
	} {
		if tags := affected[handler].node.Tags; !slices.Contains(tags, tag) {
			t.Errorf("%s should be tagged %q, tags=%v", handler, tag, tags)
		}
	}

	// price is the single chokepoint all four handlers go through, so it must
	// be the graph's fan-in maximum — that is what lifts it above the handlers
	// in severity rather than any weight tuning.
	price := affected["OrderService.price"].node
	maxInDegree := 0
	for _, n := range depGraph.Nodes {
		if n.InDegree > maxInDegree {
			maxInDegree = n.InDegree
		}
	}
	if price.InDegree != 4 || price.InDegree != maxInDegree {
		t.Errorf("OrderService.price in-degree = %d (graph max %d), want 4 and equal to the max",
			price.InDegree, maxInDegree)
	}
	for _, untouched := range []string{"InventoryService.reserve", "reserve_stock", "OrderService.reserve_inventory"} {
		if _, ok := affected[untouched]; ok {
			t.Errorf("%s is on the inventory branch and must not be affected", untouched)
		}
	}

	// price -> apply_discount goes through self.pricing, typed from the
	// `pricing: DiscountEngine` parameter (param-typed confidence), so it is
	// no longer a 0.3 duck-typed edge.
	var priceToApply []graph.Edge
	for _, e := range depGraph.OutEdges[affected["OrderService.price"].node.ID] {
		if depGraph.Nodes[e.To].QualifiedName == "DiscountEngine.apply_discount" {
			priceToApply = append(priceToApply, e)
		}
	}
	if len(priceToApply) != 1 || priceToApply[0].Confidence != graph.ConfidenceLocalInference {
		t.Errorf("OrderService.price -> apply_discount: want one edge at %.2f, got %+v",
			graph.ConfidenceLocalInference, priceToApply)
	}
}

// commitFixtureBase makes dir a fresh single-commit git repo of its current
// files, discarding any copied .git. The base state is then exactly the
// fixture files on disk, independent of whether examples/tinyshop's own repo
// has committed them (CI has no nested .git at all).
func commitFixtureBase(t *testing.T, dir string) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf("removing copied .git: %v", err)
	}
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("git init: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		t.Fatalf("git add: %v", err)
	}
	sig := &object.Signature{Name: "fixture", Email: "fixture@localhost", When: time.Unix(0, 0)}
	if _, err := wt.Commit("TinyShop fixture base", &git.CommitOptions{Author: sig}); err != nil {
		t.Fatalf("git commit: %v", err)
	}
}

// runTinyShopPipeline copies the TinyShop fixture to a temp dir, commits it as
// the base, applies the apply_discount signature change as an uncommitted
// edit, and runs parse → resolve → graph → traverse → severity, returning the
// graph and the scored blast radius.
func runTinyShopPipeline(t *testing.T) (*graph.Graph, graph.BlastRadiusResult) {
	t.Helper()

	// Find the TinyShop fixture relative to this test file
	fixturePath, err := filepath.Abs(filepath.Join("..", "..", "..", "examples", "tinyshop"))
	if err != nil {
		t.Fatalf("Failed to resolve tinyshop path: %v", err)
	}

	absPath := filepath.Join(t.TempDir(), "tinyshop")
	if err := os.CopyFS(absPath, os.DirFS(fixturePath)); err != nil {
		t.Fatalf("Failed to copy tinyshop fixture: %v", err)
	}
	commitFixtureBase(t, absPath)

	discountEnginePath := filepath.Join(absPath, "services", "pricing", "discount_engine.py")
	originalContent, err := os.ReadFile(discountEnginePath)
	if err != nil {
		t.Fatalf("Failed to read discount_engine.py: %v", err)
	}

	const originalSig = "def apply_discount(self, base_price: Money, customer_tier: str) -> Money:"
	const modifiedSig = "def apply_discount(self, base_price: Money, customer_tier: str, currency: str = \"USD\") -> Money:"
	if !strings.Contains(string(originalContent), originalSig) {
		t.Fatalf("%s/services/pricing/discount_engine.py does not contain the original apply_discount "+
			"signature; the fixture must hold the pre-change version", fixturePath)
	}

	// Make the uncommitted change in the copy only
	modifiedContent := strings.Replace(string(originalContent), originalSig, modifiedSig, 1)
	if err := os.WriteFile(discountEnginePath, []byte(modifiedContent), 0644); err != nil {
		t.Fatalf("Failed to write modified discount_engine.py: %v", err)
	}

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

	return depGraph, blastRadius
}
