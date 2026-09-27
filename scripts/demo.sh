#!/bin/sh
# Launch the real TUI against local synthetic data; no account or TDLib needed.
set -eu
cd "$(dirname "$0")/.."
bin_dir=$(mktemp -d)
trap 'rm -rf "$bin_dir"' EXIT
trap 'exit 1' HUP INT TERM
go build -o "$bin_dir/tuilegram-demo" ./cmd/demo
"$bin_dir/tuilegram-demo"
