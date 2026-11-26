#!/usr/bin/env bash
# Prints every Go fuzz target of the repository as a JSON array for the matrix of the nightly workflow,
# one entry for each:
#
#   {"name":"model-FuzzCleanPostBody","package":"./model","target":"FuzzCleanPostBody"}
#
# Go runs one fuzz target for each invocation, so each target gets an entry of its own. A target is a
# function "func FuzzName(f *testing.F)" in a _test.go file. The script fails if it finds none, because
# then the nightly fuzzing would quietly do nothing.
set -euo pipefail

cd "$(dirname "$0")/../.."

targets="$(
  grep -rHE --include='*_test.go' '^func Fuzz[A-Za-z0-9_]*\(' . |
    sed -E 's|^\./(.*)/[^/:]+_test\.go:func (Fuzz[A-Za-z0-9_]*)\(.*$|\1 \2|' |
    sort -u
)"

if [ -z "$targets" ]; then
  echo "Found no fuzz targets" >&2
  exit 1
fi

# A fuzz target in the root package has no directory, so the sed above leaves its line as it is. Fail
# on that, instead of matrixing something that cannot run.
if printf '%s\n' "$targets" | grep -qvE '^[^ ]+ Fuzz[A-Za-z0-9_]*$'; then
  echo "Could not read these as a package and a fuzz target:" >&2
  printf '%s\n' "$targets" | grep -vE '^[^ ]+ Fuzz[A-Za-z0-9_]*$' >&2
  exit 1
fi

printf '%s\n' "$targets" | awk '
  BEGIN { printf "[" }
  {
    name = $1
    gsub("/", "-", name)
    printf "%s{\"name\":\"%s-%s\",\"package\":\"./%s\",\"target\":\"%s\"}", (NR > 1 ? "," : ""), name, $2, $1, $2
  }
  END { print "]" }
'
