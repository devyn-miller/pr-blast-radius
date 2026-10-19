# Quick Start Guide

Get PR Blast Radius running in under 5 minutes.

## Choose Your Path

### Path 1: GUI Mode (1 minute) NEW!

**Visual interface** - No command line needed!

```bash
cd web-demo
npm install
npm run dev
```

Open **http://localhost:3000** and:
1. Select repository path (or use `../examples/tinyshop`)
2. Choose diff type, LLM provider
3. Click "Run Analysis" 
4. See results on the same screen!

**Everything visual** - Sliders, dropdowns, live diagrams, no typing commands!

---

### Path 2: CLI Install (5 minutes)

**Prerequisites:**
- Go 1.26.5+ (per `go-core/go.mod`) and a C compiler (tree-sitter uses cgo)
- Python 3.11+
- Git

**Steps:**

```bash
# 1. Clone
git clone https://github.com/devyn-miller/pr-blast-radius
cd pr-blast-radius

# 2. Build Go binary
cd go-core
go build -o ../bin/blastradius ./cmd/blastradius
cd ..

# 3. Install Python package
cd py-orchestrator
pip install -e .
cd ..

# 4. Run the TinyShop demo. It copies examples/tinyshop to a temp dir,
#    applies a signature change to apply_discount there, and analyzes it;
#    the fixture itself is never modified.
make demo DEMO_LLM=none

# 5. View result
cat py-orchestrator/blast-radius-output/impact.md
```

**Success!** You should see a Mermaid diagram with 6 affected nodes:
`OrderService.price` at distance 1, in its own **Critical Risk** group (every
pricing route goes through it, so it carries the graph's maximum fan-in), the
four route handlers that call it — `create_order`, `quote_order`, `price_cart`,
`checkout` — at distance 2 under **High Risk**, and `api/routes.py (module
level)` under **Medium Risk**. The inventory route is not affected.

---

## Verify Installation

### Test Go Engine
```bash
cd go-core
go test ./...

# Expected output:
# ok  	cmd/blastradius       (including integration test)
# ok  	internal/astparse
# ok  	internal/graph
# ... (all packages passing)
```

### Test Python Orchestrator
```bash
cd py-orchestrator
pytest -v

# Expected output:
# ... all tests passing
```

### Test End-to-End
```bash
# From repo root
make demo DEMO_LLM=none
```

---

## What to Try Next

### 1. Use the GUI on Your Own Repo
- Run `npm run dev` in `web-demo/`
- Enter your repo path
- Configure visually with sliders/dropdowns
- No need to remember CLI flags!

### 2. Or Use CLI to Analyze Your Own Repo
```bash
python -m orchestrator.cli scan \
  --repo /path/to/your/project \
  --diff WORKING_TREE \
  --llm auto
```

### 3. Compare Branches
```bash
python -m orchestrator.cli scan \
  --repo . \
  --diff main...feature-branch
```

### 4. Use Different LLMs
```bash
# Gemini (best quality, free tier)
export GEMINI_API_KEY="get-from-https://aistudio.google.com/apikey"
python -m orchestrator.cli scan --repo . --diff HEAD~1 --llm gemini

# Ollama (local, private)
ollama serve  # In another terminal
python -m orchestrator.cli scan --repo . --diff HEAD~1 --llm ollama

# Template-only (no LLM, always works)
python -m orchestrator.cli scan --repo . --diff HEAD~1 --llm none
```

### 5. Customize Depth
```bash
python -m orchestrator.cli scan \
  --repo . \
  --diff WORKING_TREE \
  --max-depth 5 \         # Deeper traversal
  --verbose               # Show debug logs
```

---

## Troubleshooting

### "Command not found: blastradius"
```bash
# Make sure you built the Go binary
cd go-core
go build -o ../bin/blastradius ./cmd/blastradius

# Add to PATH
export PATH=$PATH:$(pwd)/../bin
```

### "ModuleNotFoundError: No module named 'orchestrator'"
```bash
# Install Python package in editable mode
cd py-orchestrator
pip install -e .
```

### "No changes detected"
```bash
# Make sure you have uncommitted changes
git status

# Or specify a commit range
python -m orchestrator.cli scan --repo . --diff HEAD~1..HEAD
```

### Mermaid diagram not rendering in browser
- GitHub: Mermaid is natively supported in `.md` files
- VS Code: Install "Markdown Preview Mermaid Support" extension
- Browser: Use [mermaid.live](https://mermaid.live) to paste diagram code

### Python tests failing
```bash
# Install dev dependencies
pip install -e ".[dev]"

# Use a supported Python (3.11+) from the project venv
source .venv/bin/activate
python --version
python -m pytest
```

---

## Learn More

- **README.md** — Full documentation
- **Architecture** — See "How It Works" in README
- **Examples** — Browse `examples/tinyshop/`
- **Tests** — Study test files for usage patterns

---

## Portfolio Checklist

Ready to showcase this project? Complete these steps:

- [ ] Add custom domain (optional but impressive)
- [ ] Create 30-second GIF demo
- [ ] Write README with clear examples
- [ ] Add architecture diagram
- [ ] Create GitHub releases
- [ ] Write technical blog post
- [ ] Add to portfolio website
- [ ] Share on Twitter/LinkedIn

---

## Pro Tips

1. **Use `--verbose` flag** when debugging to see each pipeline stage
2. **Cache is your friend** — SQLite cache speeds up repeated analysis
3. **Start with `--llm none`** to verify Go engine works before trying LLM
4. **Max depth matters** — depth=3 is usually enough, depth=5+ can be slow
5. **Pre-commit hook** — Add analysis to your pre-commit workflow

---

## Demo Script (for presentations)

```bash
# 1. Run the demo (temp copy of TinyShop with apply_discount's signature changed)
make demo DEMO_LLM=none        # or: make demo-gemini

# 2. Show output
cat py-orchestrator/blast-radius-output/impact.md

# 3. Explain
# "OrderService.price calls apply_discount, and four POST routes call price"
# "price is critical, not the routes: it's the one chokepoint they all share"
# "The GET /inventory route is NOT affected - different dependency path"
# "Deterministic: tree-sitter AST parsing builds the diagram; the LLM only writes the notes"
```

---

**Questions?** Check [README.md](./README.md) or open an issue!

---

<div align="center">

**Star the repo if this helped you!**

[GitHub](https://github.com/devyn-miller/pr-blast-radius) • [Docs](./README.md)

</div>
