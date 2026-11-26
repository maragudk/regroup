#!/usr/bin/env bash
# Opens an issue for a failed nightly run on main, or comments on the one that is open, for the nightly
# workflow. It says which parts failed, which tests and fuzz targets in them, and where the logs and
# the failing fuzz inputs are. It reads only the logs the run kept, and runs nothing from them.
#
# Settings, from the environment:
#
#   GH_REPO, GH_TOKEN  for the gh command: the repository, and a token that may write issues
#   LABEL              the label that sets these issues apart from other issues (nightly)
#   LOGS               the directory the run's logs were downloaded to
#   RUN_URL, RUN_ID    the run's page and its ID
#   TARGETS_RESULT, HISTORY_RESULT, FUZZ_RESULT, RACE_RESULT
#                      what each job of the run ended as: success, failure, cancelled, or skipped
#
# Files in LOGS: history.log, race.log, and for each fuzz target fuzz-<package>-<target>.log, with the
# failing inputs in fuzz-<package>-<target>-inputs/.
set -euo pipefail

label="${LABEL:-nightly}"
logs="${LOGS:?LOGS is the directory the logs were downloaded to}"
run_url="${RUN_URL:?RUN_URL is the page of the run}"
run_id="${RUN_ID:?RUN_ID is the ID of the run}"

# Leaves at most this much of anything quoted, so that the issue stays within what GitHub accepts.
max_excerpt_lines=40
max_input_bytes=1500
max_issue_bytes=60000

body="$(mktemp)"
trap 'rm -f "$body"' EXIT

# Fails with something other than success, and so is worth a mention.
failed() {
  [ "${1:-}" = "failure" ]
}

# What a log says failed: the tests, the packages, the panics and the data races, and then the start of
# the first of them, or the end of the log if the run did not fail the way Go reports.
describe_log() {
  local log="$1" tests packages panics races first
  if [ ! -s "$log" ]; then
    printf '\nThere is no log for this, so the job failed before it ran the tests or while it was kept.\n'
    return
  fi

  tests="$(grep -E '^--- FAIL: ' "$log" | sed -E 's/^--- FAIL: ([^ ]+).*$/\1/' | sort -u | head -n 20 || true)"
  packages="$(grep -E '^FAIL[[:space:]]' "$log" | awk '{ print $2 }' | sort -u | head -n 20 || true)"
  panics="$(grep -cE '^panic: ' "$log" || true)"
  races="$(grep -cF 'WARNING: DATA RACE' "$log" || true)"

  if [ -n "$tests" ]; then
    printf '\nFailing tests:\n\n'
    printf '%s\n' "$tests" | sed 's/^/- `/; s/$/`/'
  fi
  if [ -n "$packages" ]; then
    printf '\nFailing packages:\n\n'
    printf '%s\n' "$packages" | sed 's/^/- `/; s/$/`/'
  fi
  if [ "${races:-0}" -gt 0 ]; then
    printf '\nThe race detector reported %s data race(s).\n' "$races"
  fi
  if [ "${panics:-0}" -gt 0 ]; then
    printf '\nThere were %s panic(s).\n' "$panics"
  fi

  first="$(grep -nE -m 1 '^(--- FAIL: |panic: |WARNING: DATA RACE)' "$log" | cut -d: -f1 || true)"
  if [ -n "$first" ]; then
    printf '\nFrom the first failure in the log:\n\n``````text\n'
    sed -n "${first},$((first + max_excerpt_lines - 1))p" "$log" | cut -c 1-300
    printf '``````\n'
  else
    printf '\nNo failing test in the log. The end of it:\n\n``````text\n'
    tail -n 30 "$log" | cut -c 1-300
    printf '``````\n'
  fi
}

printf 'The `Nightly` workflow failed on `main`: %s\n' "$run_url" >"$body"

if failed "${TARGETS_RESULT:-}"; then
  {
    printf '\n## Finding the fuzz targets\n\n'
    printf 'The job that lists the fuzz targets failed, so no target was fuzzed. The run'"'"'s page says why.\n'
  } >>"$body"
fi

if failed "${HISTORY_RESULT:-}"; then
  {
    printf '\n## Stateful history tests\n\n'
    printf 'The generated histories of the timeline, at the longer setting, failed. The log is in the run'"'"'s `nightly-history` artefact. Its log has the case hegel shrank to. Keep that as a fixed test, because CI does not save it.\n'
    describe_log "$logs/history.log"
  } >>"$body"
fi

if failed "${FUZZ_RESULT:-}"; then
  printf '\n## Fuzzing\n\n' >>"$body"
  found=""
  for log in "$logs"/fuzz-*.log; do
    [ -e "$log" ] || continue
    # A log that ends the way a failed run does.
    if grep -qE '^(--- FAIL: |FAIL[[:space:]]|panic: )' "$log"; then
      found=1
      name="$(basename "$log" .log)"
      name="${name#fuzz-}"
      {
        printf '### `%s`\n' "$name"
        printf '\nThe run'"'"'s `nightly-fuzz-%s` artefact holds the log and the failing input.\n' "$name"
        describe_log "$log"
      } >>"$body"

      inputs="$logs/fuzz-$name-inputs"
      if [ -d "$inputs" ]; then
        printed=0
        while IFS= read -r input; do
          printed=$((printed + 1))
          [ "$printed" -le 3 ] || break
          {
            printf '\nThe failing input `%s`, to keep as a seed in the fuzz target'"'"'s corpus under that path, once it is fixed:\n\n``````text\n' "${input#"$inputs"/}"
            head -c "$max_input_bytes" "$input"
            printf '\n``````\n'
          } >>"$body"
        done < <(find "$inputs" -type f | sort)
      fi
    fi
  done
  if [ -z "$found" ]; then
    printf 'A fuzz job failed, but no log shows a failing target. The run'"'"'s page says why.\n' >>"$body"
  fi
fi

if failed "${RACE_RESULT:-}"; then
  {
    printf '\n## The whole test suite under the race detector\n\n'
    printf 'The tests with `-race -shuffle <seed> -count=3` failed. The log is in the run'"'"'s `nightly-race` artefact, and its first line gives the seed to run again with.\n'
    seed="$(grep -m 1 -E '^Shuffling with the seed' "$logs/race.log" 2>/dev/null || true)"
    if [ -n "$seed" ]; then printf '\n%s\n' "$seed"; fi
    describe_log "$logs/race.log"
  } >>"$body"
fi

cat >>"$body" <<EOF

The logs are the run's \`nightly-*\` artefacts. To fetch them from a checkout:

\`\`\`sh
gh run download $run_id --pattern 'nightly-*' --dir nightly-$run_id
\`\`\`
EOF

# GitHub refuses a body of more than 65536 characters. Cut what is quoted from the end, not the start.
if [ "$(wc -c <"$body")" -gt "$max_issue_bytes" ]; then
  head -c "$max_issue_bytes" "$body" >"$body.cut"
  printf '\n``````\n\n(Cut: too long. The run'"'"'s artefacts have all of it.)\n' >>"$body.cut"
  mv "$body.cut" "$body"
fi

gh label create "$label" --color FBCA04 --description "A nightly run of the generated tests on main failed" || true

existing="$(gh issue list --label "$label" --state open --limit 1 --json number --jq '.[].number')"
if [ -n "$existing" ]; then
  gh issue comment "$existing" --body-file "$body"
else
  gh issue create --title "The nightly generated tests on main failed" --label "$label" --body-file "$body"
fi
