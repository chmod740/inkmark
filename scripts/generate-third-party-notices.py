#!/usr/bin/env python3
"""Generate the release notice from the exact Go and pnpm production graphs."""

from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys


ROOT = Path(__file__).resolve().parent.parent
NOTICE_PATH = ROOT / "THIRD_PARTY_NOTICES.txt"
MANIFESTS = (ROOT / "go.mod", ROOT / "go.sum", ROOT / "frontend/package.json", ROOT / "frontend/pnpm-lock.yaml")
GO_TARGETS = (("macOS", "darwin", "arm64"), ("Windows", "windows", "amd64"))
GO_TAGS = "desktop production wv2runtime.download"
LICENSE_NAME = re.compile(r"^(?:licen[cs]e|copying|notice|third.?party)", re.IGNORECASE)


def run(*args: str, env: dict[str, str] | None = None) -> str:
    completed = subprocess.run(
        args,
        cwd=ROOT,
        env=env,
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    return completed.stdout


def normalized_text(path: Path) -> str:
    text = path.read_text(encoding="utf-8", errors="replace").replace("\r\n", "\n")
    return "\n".join(line.rstrip() for line in text.splitlines()).rstrip() + "\n"


def manifest_digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def license_files(root: Path) -> list[Path]:
    files = [path for path in root.iterdir() if path.is_file() and LICENSE_NAME.match(path.name)]
    return sorted(files, key=lambda path: path.name.casefold())


def go_inventory() -> tuple[list[dict[str, object]], list[tuple[str, str, str]]]:
    components: dict[tuple[str, str], set[str]] = {}
    for target_name, goos, goarch in GO_TARGETS:
        env = os.environ.copy()
        env.update({"GOOS": goos, "GOARCH": goarch})
        output = run(
            "go", "list", "-tags", GO_TAGS, "-deps",
            "-f", "{{with .Module}}{{if .Version}}{{.Path}}\t{{.Version}}{{end}}{{end}}", ".",
            env=env,
        )
        for line in output.splitlines():
            if not line.strip():
                continue
            path, version = line.split("\t", 1)
            components.setdefault((path, version), set()).add(target_name)

    inventory: list[dict[str, object]] = []
    texts: list[tuple[str, str, str]] = []
    for (path, version), targets in sorted(components.items()):
        module_json = json.loads(run("go", "list", "-m", "-json", f"{path}@{version}"))
        module_root = Path(module_json["Dir"])
        files = license_files(module_root)
        if path == "github.com/wailsapp/go-webview2":
            loader = module_root / "webviewloader/LICENSE"
            if loader.is_file():
                files.append(loader)
        if not files:
            raise RuntimeError(f"no license or notice file found for {path}@{version}")
        component = f"{path}@{version}"
        inventory.append({"component": component, "targets": sorted(targets)})
        for file in files:
            texts.append((component, str(file.relative_to(module_root)), normalized_text(file)))
    return inventory, texts


def node_inventory() -> tuple[list[dict[str, str]], list[tuple[str, str, str]]]:
    raw = run("pnpm", "--dir", "frontend", "licenses", "list", "--prod", "--json")
    report = json.loads(raw)
    components: dict[tuple[str, str], dict[str, str]] = {}
    paths: dict[tuple[str, str], Path] = {}
    for expression, entries in report.items():
        for entry in entries:
            for raw_path in entry.get("paths", []):
                package_root = Path(raw_path)
                package_json = json.loads((package_root / "package.json").read_text(encoding="utf-8"))
                key = (package_json["name"], package_json["version"])
                components[key] = {
                    "component": f"{key[0]}@{key[1]}",
                    "license": expression if expression != "Unknown" else "See included license text",
                    "source": entry.get("homepage") or str(package_json.get("repository", "")),
                }
                paths[key] = package_root

    inventory = [components[key] for key in sorted(components)]
    texts: list[tuple[str, str, str]] = []
    for key in sorted(components):
        package_root = paths[key]
        files = license_files(package_root)
        if key == ("pako", "2.2.0"):
            zlib_notice = package_root / "lib/zlib/README"
            if zlib_notice.is_file():
                files.append(zlib_notice)
        if not files:
            raise RuntimeError(f"no license or notice file found for {key[0]}@{key[1]}")
        for file in files:
            texts.append((components[key]["component"], str(file.relative_to(package_root)), normalized_text(file)))
    return inventory, texts


def render() -> str:
    go_components, go_texts = go_inventory()
    node_components, node_texts = node_inventory()
    direct_node = json.loads((ROOT / "frontend/package.json").read_text(encoding="utf-8"))["dependencies"]

    output = [
        "INKMARK THIRD-PARTY SOFTWARE NOTICES AND LICENSES",
        "==================================================",
        "",
        "This document applies only to third-party software and font assets",
        "distributed with InkMark Markdown. It does not grant a license to any",
        "InkMark-authored source code, artwork, name, or other project material.",
        "The licensing status of the InkMark project itself is separate from the",
        "third-party licenses reproduced below.",
        "",
        "This file is generated from the release dependency manifests. Exact inputs:",
    ]
    output.extend(f"- {path.relative_to(ROOT)} SHA-256 {manifest_digest(path)}" for path in MANIFESTS)
    output.extend(["", "GO COMPONENTS EMBEDDED IN RELEASE BINARIES", "------------------------------------------"])
    for item in go_components:
        output.append(f"- {item['component']} [{', '.join(item['targets'])}]")

    output.extend(["", "FRONTEND PRODUCTION DEPENDENCY CLOSURE", "--------------------------------------"])
    output.append("Direct runtime dependencies: " + ", ".join(sorted(direct_node)))
    output.append("All entries below are included conservatively from pnpm's production graph;")
    output.append("entries not named above are transitive production dependencies.")
    for item in node_components:
        source = f"; {item['source']}" if item["source"] else ""
        output.append(f"- {item['component']} ({item['license']}{source})")

    output.extend([
        "",
        "KATEX FONT AND STRETCHY-GLYPH NOTICE",
        "------------------------------------",
        "Copyright (c) 2009-2010, Design Science, Inc. (www.mathjax.org)",
        "Copyright (c) 2014-2018 Khan Academy (www.khanacademy.org)",
        "Reserved Font Names are recorded in the individual KaTeX font files.",
        "These font files and associated stretchy-glyph data are distributed under",
        "the SIL Open Font License, Version 1.1, reproduced here in full:",
        "",
        normalized_text(ROOT / "scripts/licenses/OFL-1.1.txt").rstrip(),
        "",
        "COMPONENT LICENSE AND NOTICE TEXTS",
        "----------------------------------",
    ])

    grouped: dict[str, dict[str, object]] = {}
    for component, filename, text in go_texts + node_texts:
        digest = hashlib.sha256(text.encode("utf-8")).hexdigest()
        entry = grouped.setdefault(digest, {"text": text, "uses": []})
        entry["uses"].append(f"{component} ({filename})")
    for index, digest in enumerate(sorted(grouped), start=1):
        entry = grouped[digest]
        output.extend(["", f"License text {index} — SHA-256 {digest}", "Applies to:"])
        output.extend(f"- {use}" for use in sorted(set(entry["uses"])))
        output.extend(["", str(entry["text"]).rstrip()])

    return "\n".join(output).rstrip() + "\n"


def main() -> int:
    generated = render()
    if "--check" in sys.argv:
        current = NOTICE_PATH.read_text(encoding="utf-8") if NOTICE_PATH.exists() else ""
        if current != generated:
            print(f"{NOTICE_PATH.name} is missing or out of date", file=sys.stderr)
            return 1
        print(f"{NOTICE_PATH.name} is current")
        return 0
    NOTICE_PATH.write_text(generated, encoding="utf-8")
    print(f"Wrote {NOTICE_PATH} ({len(generated.encode('utf-8'))} bytes)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
