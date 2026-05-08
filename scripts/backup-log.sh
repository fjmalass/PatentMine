#!/bin/bash
set -euo pipefail

LOG_FILE="${PATENTMINE_LOG_FILE:-patentmine.log}"
BACKUP_DIR="${PATENTMINE_LOG_BACKUP_DIR:-backups/logs}"
MAX_LOG_BACKUPS="${PATENTMINE_MAX_LOG_BACKUPS:-5}"

mkdir -p "$BACKUP_DIR"

dir=$(dirname "$LOG_FILE")
base=$(basename "$LOG_FILE")
ext="${base##*.}"
stem="${base%.*}"
if [ "$stem" = "$base" ]; then
    ext=""
else
    ext=".$ext"
fi

today=$(date +%F)
current_log="${dir}/${stem}-${today}${ext}"
timestamp=$(date +%F_%H%M%S)
backup_path="${BACKUP_DIR}/${stem}-${timestamp}${ext}"

if [ ! -f "$current_log" ]; then
    echo "Error: Log file $current_log not found."
    exit 1
fi

cp "$current_log" "$backup_path"
echo "Log backup successful: $backup_path"

backup_count=$(find "$BACKUP_DIR" -maxdepth 1 -type f -name "${stem}-????-??-??_??????${ext}" | wc -l)
if [ "$backup_count" -gt "$MAX_LOG_BACKUPS" ]; then
    find "$BACKUP_DIR" -maxdepth 1 -type f -name "${stem}-????-??-??_??????${ext}" -print \
        | sort \
        | head -n -"$MAX_LOG_BACKUPS" \
        | xargs -r rm
fi
