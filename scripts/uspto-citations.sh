#!/usr/bin/env bash
# Fetches ALL forward citations (cited-by) for a patent, paginating until done.
# Prints each cited-by patent number, one per line.
# Usage: ./scripts/uspto-citations.sh <search_num> [api_key]
#   search_num  bare number without country/kind codes, e.g. 11611785

set -euo pipefail

SEARCH_NUM="${1:-}"
API_KEY="${2:-${USPTO_API_KEY:-}}"

if [[ -z "$SEARCH_NUM" || -z "$API_KEY" ]]; then
  echo "Usage: $0 <search_num> [api_key]"
  echo "       api_key also read from \$USPTO_API_KEY"
  exit 1
fi

BASE="https://api.uspto.gov"
PAGE_SIZE=25
START=0
TOTAL=0

echo "Forward citations for patent number: $SEARCH_NUM" >&2

while true; do
  URL="${BASE}/api/v1/patent/applications/search?q=forwardReferencedPatentNumber:(${SEARCH_NUM})&fields=applicationMetaData.patentNumber&rows=${PAGE_SIZE}&start=${START}"
  RESP=$(curl -s -H "X-API-KEY: ${API_KEY}" -H "Accept: application/json" "$URL")

  COUNT=$(echo "$RESP" | python3 -c \
    "import sys,json; d=json.load(sys.stdin); print(len(d.get('patentFileWrapperDataBag', [])))" 2>/dev/null || echo 0)

  echo "$RESP" | python3 -c "
import sys, json
d = json.load(sys.stdin)
for app in d.get('patentFileWrapperDataBag', []):
    num = app.get('applicationMetaData', {}).get('patentNumber', '')
    if num:
        print('US' + num if not num.startswith('US') else num)
" 2>/dev/null

  TOTAL=$((TOTAL + COUNT))
  echo "  page start=$START count=$COUNT (total so far: $TOTAL)" >&2

  if [[ "$COUNT" -lt "$PAGE_SIZE" ]]; then
    break
  fi
  START=$((START + PAGE_SIZE))
done

echo "Done. Total forward citations: $TOTAL" >&2
