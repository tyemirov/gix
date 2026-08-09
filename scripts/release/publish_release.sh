#!/usr/bin/env bash
set -euo pipefail

[[ $# -eq 0 ]] || { echo "error: make publish accepts no arguments" >&2; exit 1; }

helper="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/release_helper.py"
[[ -x "${helper}" ]] || { echo "error: release helper is not executable: ${helper}" >&2; exit 1; }

exec "${helper}" publish-prepared-release
