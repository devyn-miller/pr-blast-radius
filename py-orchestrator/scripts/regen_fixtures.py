"""Regenerate tests/fixtures/tinyshop_*.json from the TinyShop example.

The spec-builder tests run against verbatim Go output rather than a live
engine, so the fixtures have to be rebuilt whenever the TinyShop example or
Go's graph/severity output changes. Doing that by hand invites fixtures that
no longer correspond to any real run.

This reproduces the `make demo` setup exactly — copy examples/tinyshop to a
temp dir, commit it as a fresh base, apply the apply_discount signature change
as an uncommitted edit — then dumps get_dependency_graph and
compute_blast_radius (WORKING_TREE, max_depth=3) to the fixture files.

Usage (from py-orchestrator/, after `make build`):
    python scripts/regen_fixtures.py
"""
import asyncio
import json
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

from orchestrator.mcp_client import BlastRadiusMCPClient

REPO_ROOT = Path(__file__).resolve().parents[2]
TINYSHOP = REPO_ROOT / "examples" / "tinyshop"
GO_BINARY = REPO_ROOT / "bin" / "blastradius"
FIXTURES = Path(__file__).resolve().parents[1] / "tests" / "fixtures"

# Must stay in step with the Makefile's demo target and the Go golden test.
ORIGINAL_SIG = "def apply_discount(self, base_price: Money, customer_tier: str) -> Money:"
MODIFIED_SIG = ('def apply_discount(self, base_price: Money, customer_tier: str, '
                'currency: str = "USD") -> Money:')
MAX_DEPTH = 3


def git(repo: Path, *args: str) -> None:
    subprocess.run(["git", "-C", str(repo), *args], check=True,
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def stage_tinyshop(dest: Path) -> Path:
    """Copy TinyShop, commit it as the base, then make the uncommitted edit."""
    repo = dest / "tinyshop"
    shutil.copytree(TINYSHOP, repo, ignore=shutil.ignore_patterns(".git", "__pycache__"))

    git(repo, "init", "-q")
    git(repo, "add", "-A")
    git(repo, "-c", "user.name=fixture", "-c", "user.email=fixture@localhost",
        "commit", "-qm", "TinyShop fixture base")

    engine = repo / "services" / "pricing" / "discount_engine.py"
    source = engine.read_text()
    if ORIGINAL_SIG not in source:
        sys.exit(f"{engine}: apply_discount must hold the PRE-change signature:\n  {ORIGINAL_SIG}")
    engine.write_text(source.replace(ORIGINAL_SIG, MODIFIED_SIG, 1))

    return repo


def write_json(path: Path, payload) -> None:
    # Sorted keys and a trailing newline so a regeneration that changes nothing
    # produces no diff.
    path.write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n")
    print(f"wrote {path.relative_to(REPO_ROOT)}")


async def main() -> int:
    if not GO_BINARY.exists():
        sys.exit(f"{GO_BINARY} not found — run `make build` first")

    with tempfile.TemporaryDirectory() as tmp:
        repo = stage_tinyshop(Path(tmp))
        # Keep the cache out of the source tree.
        os.environ["BLASTRADIUS_CACHE_DB"] = str(Path(tmp) / "cache.db")

        async with BlastRadiusMCPClient(str(GO_BINARY), str(repo)) as client:
            graph = await client.get_dependency_graph()
            blast_radius = await client.compute_blast_radius("WORKING_TREE", MAX_DEPTH)

    # Two fields differ on every run: RepoPath is the temp dir, and
    # ExecutionTime is a wall-clock duration. Normalize both so regenerating
    # without a real change produces no diff. RepoPath becomes the fixture's
    # path relative to the repo root (test_graph_types.py asserts that value).
    graph.Metadata.RepoPath = str(TINYSHOP.relative_to(REPO_ROOT))
    blast_radius.ExecutionTime = 0

    FIXTURES.mkdir(parents=True, exist_ok=True)
    write_json(FIXTURES / "tinyshop_graph.json", graph.model_dump(mode="json"))
    write_json(FIXTURES / "tinyshop_blast_radius.json", blast_radius.model_dump(mode="json"))

    print(f"\n{len(graph.Nodes)} nodes, {blast_radius.TotalNodesAffected} affected "
          f"at max_depth={MAX_DEPTH}")
    return 0


if __name__ == "__main__":
    sys.exit(asyncio.run(main()))
