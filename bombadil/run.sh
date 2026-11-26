#!/usr/bin/env bash
# Explores the group page of a fresh preview build in a real browser with bombadil, checking the rules
# in bombadil/spec.ts, and stops the app and removes its database and build afterwards. It exits with 0
# if no rule broke, 2 if one did, and something else if the run itself failed.
#
# Settings, from the environment:
#
#   BOMBADIL_TIME_LIMIT  how long to explore, such as 30s, 3m, or 1h (3m)
#   BOMBADIL_OUTPUT      the directory to keep the trace and the logs in (.bombadil)
#   BOMBADIL_ARGS        more arguments for "bombadil browser test", such as --width 390 --height 844
#   BOMBADIL_SPEC        the specification to check, from where this is run (bombadil/spec.ts here)
#   TAILWINDCSS          the Tailwind executable to build the stylesheet with (./tailwindcss)
#
# The run stops at the first broken rule. What it keeps, in BOMBADIL_OUTPUT:
#
#   <time>.trace        every state with its screenshot, for "bombadil browser inspect"
#   <time>-bombadil.log what bombadil did and what broke
#   <time>-app.log      the app's log
#   violated.txt        the names of the rules that broke, one per line, if any did
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
spec="${BOMBADIL_SPEC:-$root/bombadil/spec.ts}"
spec="$(cd "$(dirname "$spec")" && pwd)/$(basename "$spec")"
cd "$root"

time_limit="${BOMBADIL_TIME_LIMIT:-3m}"
output="${BOMBADIL_OUTPUT:-$root/.bombadil}"
tailwindcss="${TAILWINDCSS:-./tailwindcss}"
# Unquoted where it is used, to split into arguments.
args="${BOMBADIL_ARGS:-}"

bombadil="$("$root/bombadil/install.sh")"

mkdir -p "$output"
output="$(cd "$output" && pwd)"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
trace="$output/$stamp.trace"
bombadil_log="$output/$stamp-bombadil.log"
app_log="$output/$stamp-app.log"
rm -f "$output/violated.txt"

work="$(mktemp -d "${TMPDIR:-/tmp}/regroup-bombadil.XXXXXX")"
app=""
explorer=""
cleanup() {
  for pid in $explorer $app; do
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  done
  rm -rf "$work"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

echo "Building the preview build into $work"
go build -tags preview,sqlite_fts5,sqlite_math_functions -o "$work/app" ./cmd/app
# The app reads its files from where it runs, and so would a .env there. The copy has none.
mkdir -p "$work/sqlite"
cp -R public "$work/public"
cp -R sqlite/migrations "$work/sqlite/migrations"
if ! "$tailwindcss" -i tailwind.css -o "$work/public/styles/app.css" --minify >"$work/tailwind.log" 2>&1; then
  echo "Could not build the stylesheet with $tailwindcss:" >&2
  cat "$work/tailwind.log" >&2
  exit 1
fi

# A free port, found by trying to connect to it, below the range the system hands out for connections.
port=""
for _ in $(seq 1 50); do
  candidate=$((20000 + RANDOM % 12768))
  if ! (exec 3<>"/dev/tcp/127.0.0.1/$candidate") 2>/dev/null; then
    port="$candidate"
    break
  fi
done
if [ -z "$port" ]; then
  echo "Found no free port" >&2
  exit 1
fi
origin="http://localhost:$port/"

echo "Starting it at $origin, with an empty database that it seeds"
(
  cd "$work"
  exec env \
    AWS_ENDPOINT_URL_LOCAL=false \
    BASE_URL="http://localhost:$port" \
    CSP_ALLOW_UNSAFE_INLINE=false \
    DATABASE_PATH="$work/app.db" \
    GROUP_NAME= \
    GROUP_TIME_ZONE=Europe/Copenhagen \
    LOG_LEVEL=info \
    SECURE_COOKIE=false \
    SERVER_ADDRESS="localhost:$port" \
    SERVER_ADDRESS_LOCAL=false \
    ./app
) >"$app_log" 2>&1 &
app=$!

ready=""
for _ in $(seq 1 150); do
  if curl -fs --max-time 2 -o /dev/null "$origin"; then
    ready=1
    break
  fi
  if ! kill -0 "$app" 2>/dev/null; then
    echo "The app stopped before it answered. Its log:" >&2
    cat "$app_log" >&2
    exit 1
  fi
  sleep 0.2
done
if [ -z "$ready" ]; then
  echo "The app did not answer at $origin. Its log:" >&2
  cat "$app_log" >&2
  exit 1
fi

echo "Exploring for $time_limit"
# In the background, so that a signal to this script stops it at once, through cleanup.
set -f
# shellcheck disable=SC2086
"$bombadil" browser test --headless --exit-on-violation --time-limit "$time_limit" --output-path "$trace" $args \
  "$origin" "$spec" >"$bombadil_log" 2>&1 &
explorer=$!
set +f
status=0
wait "$explorer" || status=$?
explorer=""

# The log holds every action, so only its end is shown: what broke, and how to look at it.
grep -E '^[A-Za-z0-9_$]+ was violated:$' "$bombadil_log" | sed 's/ was violated:$//' | sort -u >"$output/violated.txt" || true
if [ ! -s "$output/violated.txt" ]; then
  rm -f "$output/violated.txt"
fi
tail -n 60 "$bombadil_log"
echo
echo "The trace: $trace"
echo "What bombadil did: $bombadil_log"
echo "The app's log: $app_log"
case "$status" in
0) echo "No rule broke." ;;
2) echo "A rule broke: $(paste -sd ' ' "$output/violated.txt"). Look at it with: $bombadil browser inspect $trace" ;;
*) echo "The run failed, with exit code $status." ;;
esac
exit "$status"
