#!/usr/bin/env python3
"""Render a supervisor-bench JSON document as a Markdown table."""
from __future__ import annotations

import json
import sys
from pathlib import Path


def fmt_ms(v: float | None) -> str:
    if v is None or v == 0:
        return "—"
    if v < 1:
        return f"{v:.2f} ms"
    if v < 100:
        return f"{v:.1f} ms"
    return f"{int(round(v))} ms"


def fmt_kb(v: int | None) -> str:
    if v is None or v == 0:
        return "—"
    return f"{v / 1024:.1f} MB"


def render(doc: dict) -> str:
    rows = doc.get("results", [])
    rows.sort(key=lambda r: r.get("idle_rss_kb", 0))

    n = rows[0]["supervised_n"] if rows else 0

    lines = []
    lines.append(f"# Supervisor benchmark")
    lines.append("")
    lines.append(f"- **Run**: {doc.get('timestamp', '?')}")
    lines.append(f"- **Kernel**: `{doc.get('kernel', '?')}`")
    lines.append(f"- **Methodology**: see [`scripts/bench/README.md`](../README.md)")
    lines.append("")

    lines.append(f"| Supervisor | Version | Cold start | Idle RSS | RSS w/ {n} procs |")
    lines.append("| :--- | :--- | ---: | ---: | ---: |")
    for r in rows:
        lines.append(
            "| {sup} | `{ver}` | {cold} | {idle} | {with_n} |".format(
                sup=r.get("supervisor", "?"),
                ver=r.get("version", "?"),
                cold=fmt_ms(r.get("cold_start_ms")),
                idle=fmt_kb(r.get("idle_rss_kb")),
                with_n=fmt_kb(r.get("rss_with_n_kb")),
            )
        )

    lines.append("")
    lines.append("Raw JSON: [`results.json`](./results.json).")
    lines.append("")
    return "\n".join(lines)


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: render.py <results.json>", file=sys.stderr)
        return 2
    doc = json.loads(Path(sys.argv[1]).read_text())
    print(render(doc))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
