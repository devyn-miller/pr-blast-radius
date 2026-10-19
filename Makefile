.PHONY: init build test fixtures demo demo-gemini clean

init:
	cd go-core && go mod tidy
	cd py-orchestrator && pip install -e . && pip install -r requirements.txt

build:
	cd go-core && go build -o ../bin/blastradius ./cmd/blastradius

test:
	cd go-core && go test ./... -v
	cd py-orchestrator && pytest -v

# Rebuilds py-orchestrator/tests/fixtures/tinyshop_*.json from the TinyShop
# example by running the real Go engine over it, so the spec-builder tests
# always assert against output Go actually produced. Run this after changing
# examples/tinyshop or anything in Go's graph/severity output.
fixtures: build
	cd py-orchestrator && python scripts/regen_fixtures.py

# Copies the TinyShop fixture to a temp dir, commits the copied files as a
# fresh base (same as the golden test, so the demo doesn't depend on what
# examples/tinyshop's own repo has committed), then applies the uncommitted
# apply_discount signature change. Never modifies examples/tinyshop.
# DEMO_LLM selects the provider: auto | gemini | ollama | none.
DEMO_LLM ?= auto

demo: build
	@set -e; demo_dir=$$(mktemp -d); \
	cp -R examples/tinyshop "$$demo_dir/tinyshop"; \
	rm -rf "$$demo_dir/tinyshop/.git"; \
	git -C "$$demo_dir/tinyshop" init -q; \
	git -C "$$demo_dir/tinyshop" add -A; \
	git -C "$$demo_dir/tinyshop" -c user.name=demo -c user.email=demo@localhost commit -qm "TinyShop fixture base"; \
	engine="$$demo_dir/tinyshop/services/pricing/discount_engine.py"; \
	sed -i.orig 's/customer_tier: str) -> Money:/customer_tier: str, currency: str = "USD") -> Money:/' "$$engine"; \
	rm "$$engine.orig"; \
	grep -q 'currency: str = "USD"' "$$engine" || { echo "demo: apply_discount signature not found in $$engine" >&2; exit 1; }; \
	echo "demo: analysing $$demo_dir/tinyshop"; \
	cd py-orchestrator && BLASTRADIUS_CACHE_DB="$$demo_dir/cache.db" python -m orchestrator.cli scan \
		--repo "$$demo_dir/tinyshop" \
		--diff WORKING_TREE \
		--go-binary ../bin/blastradius \
		--llm $(DEMO_LLM) \
		--verbose

# Requires GEMINI_API_KEY in .env (or the environment).
demo-gemini:
	$(MAKE) demo DEMO_LLM=gemini

clean:
	rm -rf bin/*
	find . -name "*.db" -delete