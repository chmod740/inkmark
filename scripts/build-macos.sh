#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
project_dir="$(cd "$script_dir/.." && pwd)"
cd "$project_dir"

notice_source="$project_dir/THIRD_PARTY_NOTICES.txt"
if [[ ! -s "$notice_source" ]]; then
  echo "Missing third-party notice: $notice_source" >&2
  exit 1
fi

python3 scripts/generate-third-party-notices.py --check

if ! command -v wails >/dev/null 2>&1; then
  go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
  export PATH="$(go env GOPATH)/bin:$PATH"
fi

wails build -clean -trimpath -platform darwin/universal
app_path="$project_dir/build/bin/inkmark.app"
notice_destination="$app_path/Contents/Resources/THIRD_PARTY_NOTICES.txt"
install -m 0644 "$notice_source" "$notice_destination"
# Wails creates an ad-hoc signature. Adding a resource changes the bundle, so
# renew that signature before packaging and verify the complete app afterwards.
codesign --force --deep --sign - "$app_path"
codesign --verify --deep --strict "$app_path"
cmp -s "$notice_source" "$notice_destination"
go test ./...
pnpm --dir frontend typecheck
pnpm --dir frontend test:i18n
pnpm --dir frontend test:export
pnpm --dir frontend test:scroll
pnpm --dir frontend test:preview
pnpm --dir frontend test:markdown
pnpm --dir frontend test:diagrams
pnpm --dir frontend test:update
pnpm --dir frontend test:ui
pnpm --dir frontend test:workspace
pnpm --dir frontend test:webdav
pnpm --dir frontend test:image
pnpm --dir frontend test:saved-webdav
pnpm --dir frontend test:installer
pnpm --dir frontend test:notices
node scripts/verify-offline.mjs

echo "macOS 应用：$project_dir/build/bin/inkmark.app"
