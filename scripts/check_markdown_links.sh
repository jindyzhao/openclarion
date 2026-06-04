#!/usr/bin/env bash
# Validate relative markdown links in governed documentation.
#
# Scope:
#   - All *.md files under docs/, plus the top-level governance files:
#     README.md, CONTRIBUTING.md, GOVERNANCE.md, SECURITY.md,
#     CODE_OF_CONDUCT.md, DCO.md, MAINTAINERS.md,
#     DEVELOPMENT_WORKFLOW.md.
#
# Rules:
#   - Extract `[text](target)` markdown links.
#   - Skip absolute URLs (http://, https://, mailto:, tel:, ftp://).
#   - For relative targets:
#       * Strip any `#anchor` fragment.
#       * Resolve against the source file's directory.
#       * The resolved path must exist on disk.
#   - For Markdown targets with `#anchor` fragments:
#       * Validate the anchor against generated heading slugs or explicit
#         HTML `id` / `name` anchors in the target file.
#   - Every Markdown file under docs/ must be reachable from docs/README.md
#     through relative Markdown links.
#
# This is a string-level lint; fenced code blocks and inline code spans
# are stripped before extraction to reduce false positives.

set -euo pipefail

cd "$(dirname "$0")/.."

python3 - <<'PY'
import os
import re
import stat
import sys
import urllib.parse
from collections import defaultdict, deque
from pathlib import Path

ROOT = Path(".").resolve()

GOVERNANCE_TOP = [
    "README.md", "CONTRIBUTING.md", "GOVERNANCE.md", "SECURITY.md",
    "CODE_OF_CONDUCT.md", "DCO.md", "MAINTAINERS.md",
    "DEVELOPMENT_WORKFLOW.md",
]

docs = ROOT / "docs"
LINK_RE = re.compile(r"\[([^\]]+)\]\(([^)\s]+)\)")
HEADING_RE = re.compile(r"^(#{1,6})[ \t]+(.+?)\s*$", re.MULTILINE)
HTML_ANCHOR_RE = re.compile(r"""<a\s+[^>]*(?:id|name)=["']([^"']+)["']""", re.IGNORECASE)
SCHEME_SKIP = ("http://", "https://", "mailto:", "tel:", "ftp://")

broken = []
broken_anchors = []
regularity_errors = []
outside_targets = []
orphan_docs = []
total_links = 0
anchor_cache = {}
regularity_cache = {}
doc_edges = defaultdict(set)

def absolute_path(path):
    path = Path(path)
    if path.is_absolute():
        return path
    return ROOT / path

def display_path(path):
    path = absolute_path(path)
    try:
        return str(path.relative_to(ROOT))
    except ValueError:
        return str(path)

def symlink_path_component(path):
    path = absolute_path(path)
    try:
        rel = path.relative_to(ROOT)
    except ValueError:
        return None
    current = ROOT
    for part in rel.parts:
        current = current / part
        try:
            mode = current.lstat().st_mode
        except FileNotFoundError:
            return None
        if stat.S_ISLNK(mode):
            return current
    return None

def is_regular_markdown_file(path):
    path = absolute_path(path)
    key = str(path)
    if key in regularity_cache:
        return regularity_cache[key]

    link_component = symlink_path_component(path)
    if link_component is not None:
        regularity_errors.append(
            f"  {display_path(path)} uses symlink path component {display_path(link_component)}"
        )
        regularity_cache[key] = False
        return False

    try:
        mode = path.lstat().st_mode
    except FileNotFoundError:
        regularity_cache[key] = False
        return False

    if not stat.S_ISREG(mode):
        regularity_errors.append(f"  {display_path(path)} must be a regular markdown file")
        regularity_cache[key] = False
        return False

    regularity_cache[key] = True
    return True

file_candidates = []
if docs.exists() or docs.is_symlink():
    if docs.is_symlink() or not docs.is_dir():
        regularity_errors.append("  docs must be a real directory, not a symlink or file")
    else:
        for p in sorted(docs.rglob("*.md")):
            file_candidates.append(p)
for name in GOVERNANCE_TOP:
    p = ROOT / name
    if p.exists() or p.is_symlink():
        file_candidates.append(p)

files = [p for p in file_candidates if is_regular_markdown_file(p)]

if not files and not regularity_errors:
    print("[links-check] no markdown files; skipping.")
    sys.exit(0)

def strip_ignored_markdown(text):
    # Preserve newlines in fenced blocks so reported link line numbers stay
    # anchored to the original source file.
    text = re.sub(
        r"```.*?```",
        lambda m: "\n" * m.group(0).count("\n"),
        text,
        flags=re.DOTALL,
    )
    return re.sub(r"`[^`\n]*`", "", text)

def slugify_heading(heading):
    heading = heading.strip()
    heading = re.sub(r"\s+#+\s*$", "", heading)
    heading = re.sub(r"<[^>]+>", "", heading)
    heading = re.sub(r"[^\w\s-]", "", heading, flags=re.UNICODE)
    heading = re.sub(r"\s+", "-", heading.strip().lower())
    return heading

def markdown_anchors(path):
    path = absolute_path(path)
    if path in anchor_cache:
        return anchor_cache[path]
    text = path.read_text(encoding="utf-8")
    stripped = strip_ignored_markdown(text)
    anchors = set()
    counts = {}
    for match in HEADING_RE.finditer(stripped):
        slug = slugify_heading(match.group(2))
        if not slug:
            continue
        count = counts.get(slug, 0)
        anchors.add(slug if count == 0 else f"{slug}-{count}")
        counts[slug] = count + 1
    for match in HTML_ANCHOR_RE.finditer(stripped):
        anchor = match.group(1).strip().lower()
        if anchor:
            anchors.add(anchor)
    anchor_cache[path] = anchors
    return anchors

for src in files:
    text = src.read_text(encoding="utf-8")
    text_stripped = strip_ignored_markdown(text)

    for m in LINK_RE.finditer(text_stripped):
        target = m.group(2)
        total_links += 1
        if target.startswith(SCHEME_SKIP):
            continue
        if "#" in target:
            path_part, fragment = target.split("#", 1)
            fragment = urllib.parse.unquote(fragment).strip().lower()
        else:
            path_part, fragment = target, ""
        if path_part.startswith("/"):
            target_path = ROOT / path_part.lstrip("/")
        elif path_part:
            target_path = src.parent / path_part
        else:
            target_path = src
        resolved = target_path.resolve(strict=False)
        line_no = text_stripped.count("\n", 0, m.start()) + 1
        try:
            resolved.relative_to(ROOT)
        except ValueError:
            outside_targets.append((str(src.relative_to(ROOT)), line_no, target, str(resolved)))
            continue
        if not resolved.exists():
            try:
                rel_resolved = resolved.relative_to(ROOT)
            except ValueError:
                rel_resolved = resolved
            broken.append((str(src.relative_to(ROOT)), line_no, target, str(rel_resolved)))
            continue
        if resolved.suffix.lower() == ".md":
            if not is_regular_markdown_file(target_path):
                continue
            try:
                resolved.relative_to(docs)
            except ValueError:
                pass
            else:
                doc_edges[src.resolve(strict=False)].add(resolved.resolve(strict=False))
        if fragment and resolved.suffix.lower() == ".md":
            anchors = markdown_anchors(target_path)
            if fragment not in anchors:
                try:
                    rel_resolved = resolved.relative_to(ROOT)
                except ValueError:
                    rel_resolved = resolved
                broken_anchors.append((str(src.relative_to(ROOT)), line_no, target, str(rel_resolved), fragment))

docs_root = (docs / "README.md").resolve(strict=False)
all_doc_files = {p.resolve(strict=False) for p in files if p.resolve(strict=False).is_relative_to(docs)} if docs.is_dir() else set()
if all_doc_files:
    if not docs_root.exists():
        orphan_docs.append(("docs/README.md", "missing documentation root"))
    else:
        reachable = set()
        queue = deque([docs_root])
        while queue:
            current = queue.popleft()
            if current in reachable or current not in all_doc_files:
                continue
            reachable.add(current)
            for target in sorted(doc_edges.get(current, set())):
                if target not in reachable:
                    queue.append(target)
        for doc in sorted(all_doc_files - reachable):
            orphan_docs.append((str(doc.relative_to(ROOT)), "not reachable from docs/README.md"))

if broken or broken_anchors or regularity_errors or outside_targets or orphan_docs:
    if broken:
        print("[links-check] broken markdown links detected:", file=sys.stderr)
        for src, line, target, resolved in broken:
            print(f"  {src}:{line} -> {target}  (resolved: {resolved})", file=sys.stderr)
    if broken_anchors:
        print("[links-check] broken markdown anchors detected:", file=sys.stderr)
        for src, line, target, resolved, fragment in broken_anchors:
            print(f"  {src}:{line} -> {target}  (target file: {resolved}, missing anchor: {fragment})", file=sys.stderr)
    if regularity_errors:
        print("[links-check] non-regular markdown inputs detected:", file=sys.stderr)
        for error in regularity_errors:
            print(error, file=sys.stderr)
    if outside_targets:
        print("[links-check] relative markdown links resolve outside the repository:", file=sys.stderr)
        for src, line, target, resolved in outside_targets:
            print(f"  {src}:{line} -> {target}  (resolved: {resolved})", file=sys.stderr)
    if orphan_docs:
        print("[links-check] orphan docs detected:", file=sys.stderr)
        for doc, reason in orphan_docs:
            print(f"  {doc} ({reason})", file=sys.stderr)
    print("", file=sys.stderr)
    print("Fix: update the link target, document path, heading anchor, docs/README.md reachability chain, or replace symlink/non-regular markdown paths with regular files inside the repository.", file=sys.stderr)
    sys.exit(1)

print(f"[links-check] OK ({len(files)} files, {total_links} links scanned)")
PY
