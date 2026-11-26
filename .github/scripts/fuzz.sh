#!/usr/bin/env bash
# Fuzzes one Go fuzz target for a while, for the nightly workflow.
# Usage: .github/scripts/fuzz.sh <package> <target> <fuzz time> <output directory>
#
# It keeps the log in <output directory>/fuzz-<package>-<target>.log. If the target fails, Go writes the
# failing input under the package's testdata/fuzz/, and this copies it to <output directory> and prints it,
# so that it is in the log of the run and in the artefact. The inputs the fuzzer finds that only widen the
# coverage stay in Go's cache, not in the working tree. It exits with what go test exits with.
set -uo pipefail

if [ $# -ne 4 ]; then
  echo "Usage: $0 <package> <target> <fuzz time> <output directory>" >&2
  exit 2
fi
package="$1"
target="$2"
fuzz_time="$3"
output="$4"

# The fuzz time and the names end up in a command line and in file names, so they are what they should be.
case "$fuzz_time" in
[0-9]*[smh]) ;;
*)
  echo "The fuzz time is \"$fuzz_time\", which is not a number of seconds, minutes or hours, such as 4m" >&2
  exit 2
  ;;
esac
case "$package$target" in
*[!A-Za-z0-9_./-]*)
  echo "Not a package and a target: \"$package\" \"$target\"" >&2
  exit 2
  ;;
esac

cd "$(dirname "$0")/../.."
mkdir -p "$output"
output="$(cd "$output" && pwd)"
name="$(printf '%s-%s' "${package#./}" "$target" | tr / -)"
log="$output/fuzz-$name.log"

go test "$package" -run '^$' -fuzz "^$target\$" -fuzztime "$fuzz_time" \
  -tags sqlite_fts5,sqlite_math_functions,sqlite_foreign_keys 2>&1 | tee "$log"
status=${PIPESTATUS[0]}

if [ "$status" -ne 0 ]; then
  # New files under testdata/fuzz that git does not know about are the failing inputs.
  inputs="$(git ls-files --others --exclude-standard -- '*testdata/fuzz/*')"
  if [ -n "$inputs" ]; then
    while IFS= read -r input; do
      mkdir -p "$output/fuzz-$name-inputs/$(dirname "$input")"
      cp "$input" "$output/fuzz-$name-inputs/$input"
      printf '\n=== Failing input %s of %s ===\n' "$input" "$target" | tee -a "$log"
      cat "$input" | tee -a "$log"
    done <<<"$inputs"
  else
    printf '\n=== %s failed without leaving a failing input in testdata/fuzz ===\n' "$target" | tee -a "$log"
  fi
fi

exit "$status"
