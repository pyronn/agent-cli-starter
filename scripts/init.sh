#!/usr/bin/env bash
set -euo pipefail

project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
exec go run "${project_root}/tools/init" --root "${project_root}" "$@"
