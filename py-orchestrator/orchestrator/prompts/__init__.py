from jinja2 import Environment, FileSystemLoader
from pathlib import Path

from orchestrator.spec_builder import allowed_reasons, mermaid_id, narrative_targets

_env = Environment(loader=FileSystemLoader(Path(__file__).parent))

def render_prompt(template_name: str, **kwargs) -> str:
    return _env.get_template(template_name).render(**kwargs)


def render_narrative_prompt(spec, blast_radius, source_snippets, previous_error=None) -> str:
    """Prompt for LLM narratives over an already-built spec.

    source_snippets is keyed by Go node ID, as the pipeline fetches it.
    """
    reasons = allowed_reasons(blast_radius)
    distance = {mermaid_id(an.NodeID): an.Distance for an in blast_radius.AffectedNodes}
    source = {mermaid_id(go_id): src for go_id, src in source_snippets.items()}
    label = {n.id: n.label for n in spec.nodes}

    nodes = [
        {"id": n.id, "label": n.label, "tier": n.risk_tier, "distance": distance.get(n.id),
         "reasons": reasons.get(n.id, []), "source": source.get(n.id)}
        for n in spec.nodes
    ]
    edges = [{"from_label": label[e.from_id], "to_label": label[e.to_id], "style": e.style}
             for e in spec.edges]
    return render_prompt("mermaid_spec.j2", nodes=nodes, edges=edges,
                         targets=narrative_targets(spec), previous_error=previous_error)
