import asyncio, logging
import typer
from dotenv import find_dotenv, load_dotenv
from rich import print as rprint

from orchestrator.pipeline import Pipeline, PipelineContext
from orchestrator.mcp_client import BlastRadiusMCPClient
from orchestrator.errors import NoChangesDetectedError, MCPToolError, ProviderUnavailableError

app = typer.Typer()

@app.callback()
def main():
    # No-op: registering a callback stops Typer from collapsing a
    # single-command app into the root, so `scan` stays a subcommand.
    pass

def configure_logging(verbose: bool):
    logging.basicConfig(level=logging.DEBUG if verbose else logging.WARNING)

@app.command()
def scan(
    repo: str = typer.Option(..., help="Path to the git repository to analyze"),
    diff: str = typer.Option("HEAD~1", help="'main...feature' | 'HEAD~1' | 'WORKING_TREE'"),
    max_depth: int = typer.Option(3),
    llm: str = typer.Option("auto", help="auto | gemini | ollama | none"),
    go_binary: str = typer.Option("./bin/blastradius"),
    verbose: bool = typer.Option(False, "--verbose", "-v"),
):
    # Search upward from cwd so `make demo` (run from py-orchestrator/)
    # finds the repo-root .env. Variables already in the environment win.
    load_dotenv(find_dotenv(usecwd=True))
    configure_logging(verbose)
    asyncio.run(_run(repo, diff, max_depth, llm, go_binary))

async def _run(repo, diff, max_depth, llm, go_binary):
    async with BlastRadiusMCPClient(go_binary, repo) as client:
        pipeline = Pipeline(client, llm_preference=llm)
        ctx = PipelineContext(repo_path=repo, ref_range=diff, max_depth=max_depth)
        try:
            ctx = await pipeline.run(ctx)
        except NoChangesDetectedError as e:
            rprint(f"[yellow]{e}[/yellow]")
            raise typer.Exit(code=0)
        except MCPToolError as e:
            rprint(f"[red]Go analysis engine error: {e}[/red]")
            raise typer.Exit(code=1)
        except ProviderUnavailableError as e:
            # Only raised for an explicitly requested provider (--llm gemini /
            # ollama); `auto` falls through to the template instead.
            rprint(f"[red]LLM provider '{llm}' unavailable: {e}[/red]")
            raise typer.Exit(code=1)

    rprint(f"\n[bold]Rendered via:[/bold] {ctx.generation_outcome.provider} "
           f"({ctx.generation_outcome.attempts} attempt(s), degraded={ctx.generation_outcome.degraded})")
    rprint(f"[bold]Output:[/bold] {ctx.output_paths.markdown}")
    if ctx.output_paths.svg:
        rprint(f"[bold]Diagram:[/bold] {ctx.output_paths.svg}")

    rprint(f"\n[bold]{ctx.blast_radius.TotalNodesAffected}[/bold] nodes affected, "
           f"max depth {ctx.blast_radius.MaxDepthUsed}, "
           f"{ctx.blast_radius.TotalEdgesTraversed} edges traversed.")

    # The graph is built from the files on disk, so a deleted file has no nodes
    # and its callers cannot be traced. Say so rather than reporting a blast
    # radius that silently excludes it.
    if ctx.blast_radius.DeletedFilesNotAnalyzed:
        files = ", ".join(ctx.blast_radius.DeletedFilesNotAnalyzed)
        rprint(f"[yellow]Warning: deleted file(s) not analyzed "
               f"(no symbols in the current graph): {files}[/yellow]")

if __name__ == "__main__":
    app()