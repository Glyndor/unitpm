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

    lines = []
    lines.append("# Supervisor benchmark")
    lines.append("")
    lines.append(f"- **Run**: {doc.get('timestamp', '?')}")
    lines.append(f"- **Kernel**: `{doc.get('kernel', '?')}`")
    lines.append(f"- **Methodology**: see [`scripts/bench/README.md`](../README.md)")
    lines.append("")

    if not rows:
        lines.append("_No results._")
        lines.append("")
        return "\n".join(lines)

    tiers = sorted(int(k) for k in rows[0].get("rss_by_n", {}).keys())

    header_cells = ["Supervisor", "Version", "Cold start", "Idle RSS"]
    align_cells = [":---", ":---", "---:", "---:"]
    for n in tiers:
        header_cells.append(f"RSS @ {n}")
        align_cells.append("---:")
    lines.append("| " + " | ".join(header_cells) + " |")
    lines.append("| " + " | ".join(align_cells) + " |")

    for r in rows:
        cells = [
            r.get("supervisor", "?"),
            f"`{r.get('version', '?')}`",
            fmt_ms(r.get("cold_start_ms")),
            fmt_kb(r.get("idle_rss_kb")),
        ]
        rss_by_n = r.get("rss_by_n", {})
        for n in tiers:
            cells.append(fmt_kb(rss_by_n.get(str(n))))
        lines.append("| " + " | ".join(cells) + " |")

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
