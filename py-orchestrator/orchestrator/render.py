from orchestrator.models.mermaid_ir import MermaidDiagramSpec
import subprocess, tempfile, os
from dataclasses import dataclass

RISK_CLASS = {"critical": "riskCrit", "high": "riskHigh", "medium": "riskMed", "low": "riskLow"}

CUSTOM_STYLE_BLOCK = """
  classDef riskCrit fill:#7f1d1d,stroke:#fecaca,color:#fff,stroke-width:2px
  classDef riskHigh fill:#c2410c,stroke:#fed7aa,color:#fff
  classDef riskMed fill:#a16207,stroke:#fef08a,color:#fff
  classDef riskLow fill:#166534,stroke:#bbf7d0,color:#fff
""".strip().splitlines()

def _escape(s: str) -> str:
    return s.replace('"', "").replace("|", "").replace("[", "(").replace("]", ")").replace("\n", " ")

def compile_mermaid(spec: MermaidDiagramSpec) -> str:
    lines = [f"flowchart {spec.direction}"]
    node_by_id = {n.id: n for n in spec.nodes}

    for sg in spec.subgraphs:
        lines.append(f'  subgraph {sg.id}["{_escape(sg.title)}"]')
        for nid in sg.node_ids:
            node = node_by_id.get(nid)
            if node:
                lines.append(f'    {node.id}["{_escape(node.label)}"]:::{RISK_CLASS[node.risk_tier]}')
        lines.append("  end")

    for e in spec.edges:
        arrow = "-.->" if e.style == "dashed" else "-->"
        lines.append(f"  {e.from_id} {arrow} {e.to_id}")

    lines += CUSTOM_STYLE_BLOCK
    return "\n".join(lines)


@dataclass
class OutputPaths:
    markdown: str
    svg: str | None


async def render_all_outputs(mermaid_text: str, blast_radius, out_dir: str = "./blast-radius-output") -> OutputPaths:
    os.makedirs(out_dir, exist_ok=True)
    md_path = os.path.join(out_dir, "impact.md")
    with open(md_path, "w") as f:
        f.write("# PR Blast Radius\n\n```mermaid\n" + mermaid_text + "\n```\n")

    svg_path = os.path.join(out_dir, "impact.svg")
    try:
        with tempfile.NamedTemporaryFile(suffix=".mmd", mode="w", delete=False) as tmp:
            tmp.write(mermaid_text)
            tmp_path = tmp.name
        subprocess.run(["mmdc", "-i", tmp_path, "-o", svg_path], check=True, capture_output=True, timeout=30)
    except (FileNotFoundError, subprocess.CalledProcessError, subprocess.TimeoutExpired):
        svg_path = None  # mmdc not installed / failed — .md output still fully valid, degrade gracefully

    return OutputPaths(markdown=md_path, svg=svg_path)