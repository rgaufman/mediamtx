#!/usr/bin/env bash
set -euo pipefail

fixture_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/fixtures" && pwd)"
fixture_go="${MTX_GO:-go}"
command -v "$fixture_go" >/dev/null
export GOTOOLCHAIN=local GOPROXY=off GOWORK=off

for fixture_case in midstream-g711-h264 startup-g711-h265; do
  (
    cd -- "$fixture_root/$fixture_case"
    "$fixture_go" test -count=1 ./...
  )
done
