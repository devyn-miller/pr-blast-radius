from jinja2 import Environment, FileSystemLoader
from pathlib import Path

_env = Environment(loader=FileSystemLoader(Path(__file__).parent))

def render_prompt(template_name: str, **kwargs) -> str:
    return _env.get_template(template_name).render(**kwargs)