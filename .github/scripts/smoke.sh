#!/bin/sh
# Starts YTGrab, waits for the health endpoint, checks that a second launch reports the
# running copy instead of starting another, then stops it.
#
#   sh .github/scripts/smoke.sh path/to/ytgrab
set -eu

ytgrab="$1"
url="http://127.0.0.1:8787"
export YTGRAB_DATA_DIR="${RUNNER_TEMP:-/tmp}/ytgrab-smoke"

"$ytgrab" >server.log 2>&1 &
pid=$!
trap 'kill "$pid" 2>/dev/null || true' EXIT

i=0
until curl -fsS "$url/api/system/health" >health.json 2>/dev/null; do
    i=$((i + 1))
    if [ "$i" -ge 30 ]; then
        echo "YTGrab did not start:" >&2
        cat server.log >&2
        exit 1
    fi
    sleep 1
done
echo "health:"
cat health.json
echo

second="$("$ytgrab" 2>&1)"
echo "second launch: $second"
case "$second" in
    *"already running"*) ;;
    *) echo "second launch did not find the running copy" >&2; exit 1 ;;
esac
