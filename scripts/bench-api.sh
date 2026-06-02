#!/usr/bin/env bash
# Benchmark common REST operations against a running patentmine api.
set -euo pipefail

BASE="${PATENTMINE_API_BASE:-http://127.0.0.1:18080}"
TOKEN="${PATENTMINE_API_TOKEN:-}"
PROJECT="${PATENTMINE_BENCH_PROJECT:-}"
PATENT="${PATENTMINE_BENCH_PATENT:-US11611785B2}"

auth=()
if [[ -n "$TOKEN" ]]; then
  auth=(-H "Authorization: Bearer $TOKEN")
fi

time_curl() {
  local label="$1"
  shift
  local start end ms
  start=$(date +%s%3N)
  curl -sf "${auth[@]}" "$@" >/dev/null
  end=$(date +%s%3N)
  ms=$((end - start))
  printf "%-28s %6d ms\n" "$label" "$ms"
}

echo "PatentMine REST bench → $BASE"
echo "project=$PROJECT patent=$PATENT"
echo

time_curl "healthz" "$BASE/healthz"
LIST_URL="$BASE/patents?limit=50&offset=0&filter=fetch_state:cached"
if [[ -n "$PROJECT" ]]; then
  LIST_URL="$LIST_URL&project=$PROJECT"
fi
time_curl "patent.list (cold)" "$LIST_URL"
time_curl "patent.list (warm)" "$LIST_URL"
time_curl "patent.get" "$BASE/patents/$PATENT"
time_curl "relations cites" "$BASE/patents/$PATENT/relations?kind=cites&limit=50"
time_curl "family graph" "$BASE/patents/$PATENT/family?depth=2&limit=200"
time_curl "metricsz" "$BASE/metricsz"

echo
echo "Done. Compare with TUI using the same filter/project, then GET /metricsz."
