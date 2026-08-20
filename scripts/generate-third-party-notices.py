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
MANIFESTS = (
    ROOT / "go.mod",
    ROOT / "go.sum",
    ROOT / "frontend/package.json",
    ROOT / "frontend/pnpm-lock.yaml",
    ROOT / "frontend/pnpm-workspace.yaml",
)
GO_TARGETS = (("macOS", "darwin", "arm64"), ("Windows", "windows", "amd64"))
GO_TAGS = "desktop production wv2runtime.download"
LICENSE_NAME = re.compile(r"^(?:licen[cs]e|copying|notice|third.?party)", re.IGNORECASE)
VIZ_PACKAGE = ("@viz-js/viz", "3.29.0")
VIZ_SOURCE_REVISION = "96be4ed32789125f00cffa3ae390bd8809fc9c03"
VIZ_WRAPPER_LICENSE = {
    "path": ROOT / "scripts/licenses/viz-js-3.29.0-MIT.txt",
    "sha256": "f1fb91bf7cbcb42b2af56949e4fa909e1979d5b57c2826cabacdb4b631457720",
    "source": "https://github.com/mdaines/viz-js/blob/release-viz-3.29.0/LICENSE",
}
VIZ_EMBEDDED_COMPONENTS = (
    {
        "name": "Graphviz",
        "version": "15.1.1",
        "license": "EPL-2.0",
        "uri": "https://gitlab.com/api/v4/projects/4207231/packages/generic/graphviz-releases/15.1.1/graphviz-15.1.1.tar.gz",
        "archive_sha256": "d511d8938bbfe985b84506c96d1bbf1acba4ad88e62b51421a8b954e572f30d9",
        "license_path": ROOT / "scripts/licenses/Graphviz-15.1.1-EPL-2.0.txt",
        # The upstream tag's COPYING has no final newline; normalized_text adds one.
        "license_sha256": "8c349f80764d0648e645f41ef23772a70c995a0924b5235f735f4a3d09df127c",
        "license_source": "https://gitlab.com/graphviz/graphviz/-/blob/15.1.1/COPYING",
    },
    {
        "name": "Expat",
        "version": "2.8.1",
        "license": "MIT",
        "uri": "https://github.com/libexpat/libexpat/releases/download/R_2_8_1/expat-2.8.1.tar.gz",
        "archive_sha256": "a52eb72108be160e190b5cafa5bba8663f1313f2013e26060d1c18e26e31067b",
        "license_path": ROOT / "scripts/licenses/Expat-2.8.1-MIT.txt",
        "license_sha256": "31b15de82aa19a845156169a17a5488bf597e561b2c318d159ed583139b25e87",
        "license_source": "https://github.com/libexpat/libexpat/blob/R_2_8_1/expat/COPYING",
    },
)
FUSION_PIXEL_FONT = {
    "component": "Fusion Pixel Font 12px Monospaced 2026.07.20 (Simplified and Traditional Chinese)",
    "license": "SIL Open Font License 1.1",
    "source": "https://github.com/TakWolf/fusion-pixel-font/releases/tag/2026.07.20",
    "archive": "fusion-pixel-font-12px-monospaced-otf.woff2-v2026.07.20.zip",
    "archive_sha256": "83ff3c1fa8dd0ecd1b010c4bb256a7318ca4fb2dcaadbfdfc0ea560da219a565",
    "licenses": (
        {
            "path": ROOT / "scripts/licenses/FusionPixelFont-2026.07.20-OFL-1.1.txt",
            "archive_path": "OFL.txt",
            "sha256": "bc518cf64b8032c07690f33cc270c35c179255a6ac8efa7c165ebae7e8f76a63",
        },
        {
            "path": ROOT / "scripts/licenses/FusionPixelFont-2026.07.20-ArkPixel-OFL-1.1.txt",
            "archive_path": "LICENSES/ark-pixel/OFL.txt",
            "sha256": "3ab41567e68e3988ba1ef16dd2644eca95ca5648ea12e7d46e6287fc0bbe5aee",
        },
        {
            "path": ROOT / "scripts/licenses/FusionPixelFont-2026.07.20-Cubic11-OFL-1.1.txt",
            "archive_path": "LICENSES/cubic-11/OFL.txt",
            # The archive file has one trailing space; the vendored notice is
            # normalized without changing its license text.
            "sha256": "bdd640c94530f5845de621089875aefcaec17585dbd4dab191c97118539bf92f",
        },
        {
            "path": ROOT / "scripts/licenses/FusionPixelFont-2026.07.20-Galmuri-OFL-1.1.txt",
            "archive_path": "LICENSES/galmuri/LICENSE.txt",
            "sha256": "86a3ee9495f942f0243f18c103da9faca27adb88142613edb8bb852e56c892c1",
        },
    ),
    "assets": (
        {
            "path": ROOT / "frontend/src/assets/fonts/fusion-pixel-12px-monospaced-zh_hans.otf.woff2",
            "size": 659_528,
            "sha256": "7a08aa2bf8970d10bc0f902e9256b86628f17e119efcd9bbcd6c4a169290d867",
        },
        {
            "path": ROOT / "frontend/src/assets/fonts/fusion-pixel-12px-monospaced-zh_hant.otf.woff2",
            "size": 666_996,
            "sha256": "b42690df31fa8d0aa7b61f1508ddf3416c36c914e8329bd664a8878d29a7056d",
        },
    ),
}


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


def verified_license_text(path: Path, expected_sha256: str) -> str:
    if not path.is_file():
        raise RuntimeError(f"vendored license text is missing: {path.relative_to(ROOT)}")
    text = normalized_text(path)
    digest = hashlib.sha256(text.encode("utf-8")).hexdigest()
    if digest != expected_sha256:
        raise RuntimeError(
            f"vendored license text changed: {path.relative_to(ROOT)} "
            f"(expected {expected_sha256}, got {digest})"
        )
    return text


def verified_binary_asset(path: Path, expected_size: int, expected_sha256: str) -> None:
    if not path.is_file():
        raise RuntimeError(f"vendored binary asset is missing: {path.relative_to(ROOT)}")
    size = path.stat().st_size
    digest = manifest_digest(path)
    if size != expected_size or digest != expected_sha256:
        raise RuntimeError(
            f"vendored binary asset changed: {path.relative_to(ROOT)} "
            f"(expected {expected_size} bytes / {expected_sha256}, got {size} bytes / {digest})"
        )


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


def viz_license_texts(package_root: Path) -> list[tuple[str, str, str]]:
    provenance_path = package_root / "lib/provenance.json"
    if not provenance_path.is_file():
        raise RuntimeError("@viz-js/viz has no build provenance")
    provenance = json.loads(provenance_path.read_text(encoding="utf-8"))

    build_root = provenance.get("predicate", {}).get("buildDefinition", {}).get("externalParameters", {}).get("request", {}).get("root", {})
    revision = build_root.get("request", {}).get("args", {}).get("vcs:revision")
    if revision != VIZ_SOURCE_REVISION:
        raise RuntimeError(f"unexpected @viz-js/viz source revision: {revision!r}")

    dependencies = provenance.get("predicate", {}).get("buildDefinition", {}).get("resolvedDependencies", [])
    resolved = {
        item.get("uri"): item.get("digest", {}).get("sha256")
        for item in dependencies
        if isinstance(item, dict)
    }
    for embedded in VIZ_EMBEDDED_COMPONENTS:
        actual_digest = resolved.get(embedded["uri"])
        if actual_digest != embedded["archive_sha256"]:
            raise RuntimeError(
                f"@viz-js/viz provenance mismatch for {embedded['name']}@{embedded['version']}: "
                f"expected {embedded['archive_sha256']}, got {actual_digest!r}"
            )

    subject_digests = {
        item.get("name"): item.get("digest", {}).get("sha256")
        for item in provenance.get("subject", [])
        if isinstance(item, dict)
    }
    backend_path = package_root / "lib/backend.js"
    expected_backend_digest = subject_digests.get("backend.js")
    if not expected_backend_digest or manifest_digest(backend_path) != expected_backend_digest:
        raise RuntimeError("@viz-js/viz backend does not match its provenance subject")

    component = f"{VIZ_PACKAGE[0]}@{VIZ_PACKAGE[1]}"
    texts = [(
        component,
        f"{VIZ_WRAPPER_LICENSE['path'].relative_to(ROOT)} (official license: {VIZ_WRAPPER_LICENSE['source']})",
        verified_license_text(VIZ_WRAPPER_LICENSE["path"], VIZ_WRAPPER_LICENSE["sha256"]),
    )]
    for embedded in VIZ_EMBEDDED_COMPONENTS:
        texts.append((
            f"{component} / embedded {embedded['name']}@{embedded['version']}",
            f"{embedded['license_path'].relative_to(ROOT)} (official license: {embedded['license_source']})",
            verified_license_text(embedded["license_path"], embedded["license_sha256"]),
        ))
    return texts


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

    if VIZ_PACKAGE in components:
        components[VIZ_PACKAGE]["license"] = "MIT; embeds Graphviz 15.1.1 (EPL-2.0) and Expat 2.8.1 (MIT)"
        components[VIZ_PACKAGE]["source"] = "https://github.com/mdaines/viz-js/tree/release-viz-3.29.0/packages/viz"

    inventory = [components[key] for key in sorted(components)]
    if VIZ_PACKAGE in components:
        inventory.extend({
            "component": f"{VIZ_PACKAGE[0]}@{VIZ_PACKAGE[1]} embedded {embedded['name']}@{embedded['version']}",
            "license": embedded["license"],
            "source": embedded["uri"],
        } for embedded in VIZ_EMBEDDED_COMPONENTS)
        inventory.sort(key=lambda item: item["component"])
    texts: list[tuple[str, str, str]] = []
    for key in sorted(components):
        package_root = paths[key]
        files = license_files(package_root)
        if key == VIZ_PACKAGE:
            # The published npm archive omits its repository LICENSE and embeds
            # native WASM components. Verify SLSA provenance instead of trusting
            # package.json's MIT field, then use exact texts pinned from upstream.
            texts.extend(viz_license_texts(package_root))
            continue
        if key == ("echarts", "6.1.0"):
            d3_license = package_root / "licenses/LICENSE-d3"
            if not d3_license.is_file():
                raise RuntimeError("echarts@6.1.0 is missing licenses/LICENSE-d3")
            files.append(d3_license)
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

    output.extend(["", "BUNDLED APPLICATION FONT ASSETS", "-------------------------------"])
    output.append(
        f"- {FUSION_PIXEL_FONT['component']} ({FUSION_PIXEL_FONT['license']}; "
        f"{FUSION_PIXEL_FONT['source']})"
    )
    output.append(
        f"  Audited release archive: {FUSION_PIXEL_FONT['archive']} "
        f"SHA-256 {FUSION_PIXEL_FONT['archive_sha256']}"
    )
    for asset in FUSION_PIXEL_FONT["assets"]:
        verified_binary_asset(asset["path"], asset["size"], asset["sha256"])
        output.append(
            f"  - {asset['path'].relative_to(ROOT)} ({asset['size']} bytes; SHA-256 {asset['sha256']})"
        )

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
    font_texts = [(
        FUSION_PIXEL_FONT["component"],
        f"{license_item['path'].relative_to(ROOT)} "
        f"(release archive {license_item['archive_path']}; {FUSION_PIXEL_FONT['source']})",
        verified_license_text(license_item["path"], license_item["sha256"]),
    ) for license_item in FUSION_PIXEL_FONT["licenses"]]
    for component, filename, text in go_texts + node_texts + font_texts:
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
