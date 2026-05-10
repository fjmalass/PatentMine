#!/usr/bin/env bash
# Replicates the full USPTO ODP fetch sequence for a single patent.
# Usage: ./scripts/uspto-fetch.sh <patent_number> [api_key]
#   patent_number  e.g. US11611785B2  or  11611785  or  11611785B2
#   api_key        falls back to $USPTO_API_KEY env var

set -euo pipefail

PATENT="${1:-}"
API_KEY="${2:-${USPTO_API_KEY:-}}"

if [[ -z "$PATENT" || -z "$API_KEY" ]]; then
  echo "Usage: $0 <patent_number> [api_key]"
  echo "       api_key also read from \$USPTO_API_KEY"
  exit 1
fi

BASE="https://api.uspto.gov"

# Strip country prefix (US) and kind code (B2, A1, …)
SEARCH_NUM=$(echo "$PATENT" | tr '[:lower:]' '[:upper:]' \
  | sed 's/^US//' \
  | sed 's/[A-Z][0-9]*$//')

echo "=== 1. Search by patent number: $SEARCH_NUM ==="
SEARCH_URL="${BASE}/api/v1/patent/applications/search?q=patentNumber:(${SEARCH_NUM})"
SEARCH_RESP=$(curl -s -H "X-API-KEY: ${API_KEY}" -H "Accept: application/json" "$SEARCH_URL")
echo "$SEARCH_RESP" | python3 -m json.tool 2>/dev/null || echo "$SEARCH_RESP"

APP_NUM=$(echo "$SEARCH_RESP" | python3 -c \
  "import sys,json; d=json.load(sys.stdin); print(d['patentFileWrapperDataBag'][0]['applicationNumberText'])" 2>/dev/null || true)

if [[ -z "$APP_NUM" ]]; then
  echo "ERROR: could not extract applicationNumberText from search response"
  exit 1
fi

echo ""
echo "=== 2. Full application data: appNum=$APP_NUM ==="
APP_URL="${BASE}/api/v1/patent/applications/${APP_NUM}"
curl -s -H "X-API-KEY: ${API_KEY}" -H "Accept: application/json" "$APP_URL" \
  | python3 -m json.tool 2>/dev/null

echo ""
echo "=== 3. Continuity (family edges): appNum=$APP_NUM ==="
CONT_URL="${BASE}/api/v1/patent/applications/${APP_NUM}/continuity"
curl -s -H "X-API-KEY: ${API_KEY}" -H "Accept: application/json" "$CONT_URL" \
  | python3 -m json.tool 2>/dev/null

echo ""
echo "=== 4. Forward citations page 1 (cited by): patentNum=$SEARCH_NUM ==="
FWD_URL="${BASE}/api/v1/patent/applications/search?q=forwardReferencedPatentNumber:(${SEARCH_NUM})&fields=applicationMetaData.patentNumber&rows=25&start=0"
curl -s -H "X-API-KEY: ${API_KEY}" -H "Accept: application/json" "$FWD_URL" \
  | python3 -m json.tool 2>/dev/null
