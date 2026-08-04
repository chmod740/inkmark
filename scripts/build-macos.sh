#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
project_dir="$(cd "$script_dir/.." && pwd)"
cd "$project_dir"

if ! command -v wails >/dev/null 2>&1; then
  go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
  export PATH="$(go env GOPATH)/bin:$PATH"
fi

wails build -clean -trimpath -platform darwin/universal
go test ./...
pnpm --dir frontend typecheck
pnpm --dir frontend test:i18n
pnpm --dir frontend test:export
pnpm --dir frontend test:scroll
pnpm --dir frontend test:ui
pnpm --dir frontend test:installer
node scripts/verify-offline.mjs

echo "macOS 应用：$project_dir/build/bin/inkmark.app"
