#!/usr/bin/env python3
"""
Scan all .jsonl files under ~/.claude/projects/ and build a comprehensive
schema of every field and type encountered, grouped by top-level "type" value.

Limits:
  - At most 50 files (newest first by mtime)
  - At most 500 lines per file
"""

import json
import os
import sys
from collections import defaultdict
from pathlib import Path


def type_name(val):
    """Return a concise type string for a Python value."""
    if val is None:
        return "null"
    if isinstance(val, bool):
        return "bool"
    if isinstance(val, int):
        return "int"
    if isinstance(val, float):
        return "float"
    if isinstance(val, str):
        return "string"
    if isinstance(val, list):
        return "array"
    if isinstance(val, dict):
        return "object"
    return type(val).__name__


def collect_files(root: Path, max_files: int = 50):
    """Recursively find .jsonl files, return newest-first up to max_files."""
    files = []
    for p in root.rglob("*.jsonl"):
        if p.is_file():
            try:
                files.append((p, p.stat().st_mtime))
            except OSError:
                pass
    files.sort(key=lambda t: t[1], reverse=True)
    return [f for f, _ in files[:max_files]]


def main():
    root = Path.home() / ".claude" / "projects"
    if not root.is_dir():
        print(json.dumps({"error": f"{root} is not a directory"}), file=sys.stderr)
        sys.exit(1)

    files = collect_files(root, max_files=50)
    print(f"Found {len(files)} .jsonl files to scan", file=sys.stderr)

    MAX_LINES = 500

    # ── Per entry-type accumulators ──────────────────────────────────

    # entry_type -> field_name -> set of value types
    top_keys_by_type = defaultdict(lambda: defaultdict(set))

    # entry_type -> field_name -> set of value types  (for message sub-object)
    message_keys_by_type = defaultdict(lambda: defaultdict(set))

    # entry_type -> "string" | "array"  (content shape)
    content_shape_by_type = defaultdict(set)

    # entry_type -> block_type_name -> field_name -> set of value types
    content_block_fields = defaultdict(lambda: defaultdict(lambda: defaultdict(set)))

    # Track occurrences per type
    type_occurrence_count = defaultdict(int)

    # Global counters
    total_lines = 0
    parse_errors = 0
    files_scanned = 0

    for fpath in files:
        files_scanned += 1
        try:
            with open(fpath, "r", encoding="utf-8", errors="replace") as fh:
                for line_no, raw in enumerate(fh, 1):
                    if line_no > MAX_LINES:
                        break
                    raw = raw.strip()
                    if not raw:
                        continue
                    total_lines += 1
                    try:
                        obj = json.loads(raw)
                    except json.JSONDecodeError:
                        parse_errors += 1
                        continue

                    if not isinstance(obj, dict):
                        continue

                    entry_type = obj.get("type", "__no_type__")
                    if not isinstance(entry_type, str):
                        entry_type = str(entry_type)

                    type_occurrence_count[entry_type] += 1

                    # ── Top-level keys ───────────────────────────────
                    for k, v in obj.items():
                        top_keys_by_type[entry_type][k].add(type_name(v))

                    # ── message sub-object ───────────────────────────
                    msg = obj.get("message")
                    if isinstance(msg, dict):
                        for k, v in msg.items():
                            message_keys_by_type[entry_type][k].add(type_name(v))

                        content = msg.get("content")
                        if isinstance(content, str):
                            content_shape_by_type[entry_type].add("string")
                        elif isinstance(content, list):
                            content_shape_by_type[entry_type].add("array")
                            for block in content:
                                if isinstance(block, dict):
                                    btype = block.get("type", "__no_type__")
                                    if not isinstance(btype, str):
                                        btype = str(btype)
                                    for k, v in block.items():
                                        content_block_fields[entry_type][btype][k].add(type_name(v))
                                elif isinstance(block, str):
                                    content_block_fields[entry_type]["__plain_string__"]["_value"].add("string")
                                else:
                                    content_block_fields[entry_type]["__other__"]["_value"].add(type_name(block))
                        elif content is not None:
                            content_shape_by_type[entry_type].add(type_name(content))

        except OSError as exc:
            print(f"Warning: could not read {fpath}: {exc}", file=sys.stderr)

    # ── Build the report ─────────────────────────────────────────────
    def sets_to_lists(d):
        """Recursively convert sets to sorted lists for JSON output."""
        if isinstance(d, set):
            return sorted(d)
        if isinstance(d, dict):
            return {k: sets_to_lists(v) for k, v in sorted(d.items())}
        if isinstance(d, list):
            return [sets_to_lists(i) for i in d]
        return d

    report = {
        "_meta": {
            "files_scanned": files_scanned,
            "total_lines_parsed": total_lines,
            "parse_errors": parse_errors,
            "unique_entry_types": sorted(top_keys_by_type.keys()),
        },
        "entry_types": {},
    }

    for etype in sorted(top_keys_by_type.keys()):
        entry = {
            "occurrences": type_occurrence_count[etype],
            "top_level_fields": sets_to_lists(dict(top_keys_by_type[etype])),
        }
        if etype in message_keys_by_type:
            entry["message_fields"] = sets_to_lists(dict(message_keys_by_type[etype]))
        if etype in content_shape_by_type:
            entry["content_shapes"] = sorted(content_shape_by_type[etype])
        if etype in content_block_fields:
            entry["content_block_types"] = sets_to_lists(dict(content_block_fields[etype]))
        report["entry_types"][etype] = entry

    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    main()
