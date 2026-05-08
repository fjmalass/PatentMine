#!/bin/bash
set -euo pipefail

LOG_FILE="${PATENTMINE_LOG_FILE:-./patentmine.log}"
LINES="${PATENTMINE_LOG_LINES:-80}"

dir=$(dirname "$LOG_FILE")
base=$(basename "$LOG_FILE")
ext="${base##*.}"
stem="${base%.*}"
if [ "$stem" = "$base" ]; then
    ext=""
else
    ext=".$ext"
fi

current_log="${dir}/${stem}-$(date +%F)${ext}"

if [ ! -f "$current_log" ]; then
    echo "Error: Log file $current_log not found."
    exit 1
fi

tail -n "$LINES" "$current_log"
