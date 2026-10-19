"""The risk tier cutoffs must fit inside the severity score's reachable range.

Go's severity score is in [0, 1], but an AFFECTED node is always at distance
>= 1 (distance 0 is a changed root, which is never scored), so the distance
term can never exceed half its weight. That makes the real ceiling lower than
1.0 — and a cutoff above the ceiling defines a tier no node can ever land in.

Rather than hardcoding the ceiling, these tests re-derive it from the weights
in the Go source, so retuning the weights moves the ceiling and fails here if a
cutoff no longer fits under it.
"""
import re
from pathlib import Path

import pytest

from orchestrator.spec_builder import (
    CRITICAL_CUTOFF, HIGH_CUTOFF, MEDIUM_CUTOFF, LOWEST_TIER, risk_tier,
)

SCORER_GO = Path(__file__).resolve().parents[2] / "go-core" / "internal" / "severity" / "scorer.go"

# The smallest distance an affected node can have.
MIN_AFFECTED_DISTANCE = 1


def go_default_weights() -> dict[str, float]:
    """Parse DefaultWeights out of scorer.go — the single source of truth."""
    source = SCORER_GO.read_text()
    block = re.search(r"DefaultWeights\s*=\s*Weights\{(.*?)\}", source, re.DOTALL)
    assert block, f"DefaultWeights literal not found in {SCORER_GO}"
    weights = {
        name: float(value)
        for name, value in re.findall(r"(\w+):\s*([0-9.]+)", block.group(1))
    }
    assert set(weights) == {"Distance", "FanIn", "RouteHandler", "Confidence"}, (
        f"unexpected weight fields {sorted(weights)}; "
        "the severity formula changed, so these cutoffs need re-deriving"
    )
    return weights


def max_reachable_score(is_route: bool) -> float:
    """Mirror of severity.MaxReachableScore in Go."""
    w = go_default_weights()
    score = (
        w["Distance"] * (1.0 / (1.0 + MIN_AFFECTED_DISTANCE))
        + w["FanIn"]
        + w["Confidence"]
    )
    if is_route:
        score += w["RouteHandler"]
    return min(score, 1.0)


def test_critical_cutoff_is_reachable_by_a_non_route_node():
    ceiling = max_reachable_score(is_route=False)
    assert CRITICAL_CUTOFF <= ceiling, (
        f"critical cutoff {CRITICAL_CUTOFF} is above the maximum score a "
        f"non-route node at distance>=1 can reach ({ceiling:.4f}), so no such "
        "node can ever be critical — lower the cutoff or retune the weights"
    )


def test_every_cutoff_is_reachable():
    ceiling = max_reachable_score(is_route=False)
    for name, cutoff in [("critical", CRITICAL_CUTOFF), ("high", HIGH_CUTOFF),
                         ("medium", MEDIUM_CUTOFF)]:
        assert cutoff <= ceiling, f"{name} cutoff {cutoff} exceeds ceiling {ceiling:.4f}"


def test_cutoffs_are_strictly_descending():
    assert CRITICAL_CUTOFF > HIGH_CUTOFF > MEDIUM_CUTOFF > 0


@pytest.mark.parametrize("score,want", [
    (1.0, "critical"),
    (CRITICAL_CUTOFF, "critical"),
    (CRITICAL_CUTOFF - 0.01, "high"),
    (HIGH_CUTOFF, "high"),
    (HIGH_CUTOFF - 0.01, "medium"),
    (MEDIUM_CUTOFF, "medium"),
    (MEDIUM_CUTOFF - 0.01, LOWEST_TIER),
    (0.0, LOWEST_TIER),
])
def test_risk_tier_boundaries(score, want):
    assert risk_tier(score) == want
