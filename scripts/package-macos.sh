#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
project_dir="$(cd "$script_dir/.." && pwd)"
cd "$project_dir"

app_path="$project_dir/build/bin/inkmark.app"
if [[ ! -d "$app_path" ]]; then
  echo "Missing macOS application: $app_path" >&2
  exit 1
fi

architectures="$(lipo -archs "$app_path/Contents/MacOS/inkmark")"
if [[ "$architectures" != *"arm64"* || "$architectures" != *"x86_64"* ]]; then
  echo "The release application must be universal; found: $architectures" >&2
  exit 1
fi

version="$(node -e "const fs=require('fs'); process.stdout.write(JSON.parse(fs.readFileSync('wails.json','utf8')).info.productVersion)")"
release_dir="$project_dir/dist"
stage_dir="$(mktemp -d)"
trap 'rm -rf -- "$stage_dir"' EXIT

mkdir -p "$release_dir"
dmg_path="$release_dir/InkMark-Markdown-$version-macos-universal.dmg"
zip_path="$release_dir/InkMark-Markdown-$version-macos-universal.zip"

ditto "$app_path" "$stage_dir/InkMark Markdown.app"
ln -s /Applications "$stage_dir/Applications"

hdiutil create \
  -volname "InkMark Markdown" \
  -srcfolder "$stage_dir" \
  -fs HFS+ \
  -format UDZO \
  -ov "$dmg_path"

ditto -c -k --sequesterRsrc --keepParent "$app_path" "$zip_path"
hdiutil verify "$dmg_path"

(
  cd "$release_dir"
  shasum -a 256 "$(basename "$dmg_path")" "$(basename "$zip_path")" > SHA256SUMS
)

echo "macOS DMG: $dmg_path"
echo "macOS ZIP: $zip_path"
echo "Checksums: $release_dir/SHA256SUMS"
