#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
project_dir="$(cd "$script_dir/.." && pwd)"
cd "$project_dir"

app_path="$project_dir/build/bin/inkmark.app"
notice_source="$project_dir/THIRD_PARTY_NOTICES.txt"
notice_in_app="$app_path/Contents/Resources/THIRD_PARTY_NOTICES.txt"
if [[ ! -d "$app_path" ]]; then
  echo "Missing macOS application: $app_path" >&2
  exit 1
fi
if [[ ! -s "$notice_source" || ! -f "$notice_in_app" ]] || ! cmp -s "$notice_source" "$notice_in_app"; then
  echo "The macOS application is missing the current third-party notice" >&2
  exit 1
fi
codesign --verify --deep --strict "$app_path"

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
install -m 0644 "$notice_source" "$release_dir/THIRD_PARTY_NOTICES.txt"
dmg_path="$release_dir/InkMark-Markdown-$version-macos-universal.dmg"
zip_path="$release_dir/InkMark-Markdown-$version-macos-universal.zip"
pkg_path="$release_dir/InkMark-Markdown-$version-macos-universal.pkg"

ditto "$app_path" "$stage_dir/InkMark Markdown.app"
ln -s /Applications "$stage_dir/Applications"

# The system Installer package is the preferred update asset. It replaces the
# existing app bundle in /Applications after the running version has closed.
pkgbuild \
  --component "$stage_dir/InkMark Markdown.app" \
  --install-location /Applications \
  --identifier "com.chmod740.inkmark.pkg" \
  --version "$version" \
  "$pkg_path"

hdiutil create \
  -volname "InkMark Markdown" \
  -srcfolder "$stage_dir" \
  -fs HFS+ \
  -format UDZO \
  -ov "$dmg_path"

ditto -c -k --sequesterRsrc --keepParent "$stage_dir/InkMark Markdown.app" "$zip_path"
hdiutil verify "$dmg_path"

checksum_files=()
checksum_files+=("THIRD_PARTY_NOTICES.txt")
for artifact in "$release_dir"/InkMark-Markdown-"$version"-*; do
  [[ -f "$artifact" ]] || continue
  case "$artifact" in
    *.pkg|*.dmg|*.zip|*.exe|*.msi|*.AppImage|*.deb|*.rpm)
      checksum_files+=("$(basename "$artifact")")
      ;;
  esac
done
if [[ ${#checksum_files[@]} -eq 0 ]]; then
  echo "No release artifacts found for checksum generation" >&2
  exit 1
fi
(
  cd "$release_dir"
  shasum -a 256 "${checksum_files[@]}" > SHA256SUMS
)

echo "macOS PKG: $pkg_path"
echo "macOS DMG: $dmg_path"
echo "macOS ZIP: $zip_path"
echo "Checksums: $release_dir/SHA256SUMS"
