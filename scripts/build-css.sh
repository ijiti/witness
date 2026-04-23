#!/usr/bin/env bash
# Rebuild internal/web/static/tailwind.css from tailwind.input.css.
# The output CSS is committed to the repo so the Go binary embeds it via
# go:embed; rebuild only when templates or tailwind.config.js change.
#
# Requires: node + npx on PATH. The CSS itself is the only build-time JS dep;
# the runtime binary remains a pure-Go static.
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION=3.4.17

npx --yes "tailwindcss@${VERSION}" \
  -i tailwind.input.css \
  -o internal/web/static/tailwind.css \
  --minify

# Re-prepend the license banner (BSD 2-Clause / MIT compliance: keep notices in distributed files).
tmp=$(mktemp)
{
  echo "/*! tailwindcss v${VERSION} | MIT License | (c) Tailwind Labs, Inc. | https://tailwindcss.com */"
  cat internal/web/static/tailwind.css
} > "$tmp"
mv "$tmp" internal/web/static/tailwind.css

bytes=$(wc -c < internal/web/static/tailwind.css)
echo "wrote internal/web/static/tailwind.css (${bytes} bytes)"
