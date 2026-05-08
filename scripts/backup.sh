#!/bin/bash

# Configuration
BACKUP_DIR="backups"
DB_FILE="patentmine.db"
PDF_DIR="pdfs"
MAX_BACKUPS=10

mkdir -p "$BACKUP_DIR"

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_NAME="patentmine_backup_${TIMESTAMP}.tar.gz"
BACKUP_PATH="${BACKUP_DIR}/${BACKUP_NAME}"

# Verify database exists
if [ ! -f "$DB_FILE" ]; then
    echo "Error: Database file $DB_FILE not found."
    exit 1
fi

# Prepare list of files/dirs to backup
FILES_TO_BACKUP=("$DB_FILE")
if [ -d "$PDF_DIR" ]; then
    FILES_TO_BACKUP+=("$PDF_DIR")
fi

echo "Creating backup: $BACKUP_PATH..."

# Create compressed archive
tar -czf "$BACKUP_PATH" "${FILES_TO_BACKUP[@]}"

if [ $? -eq 0 ]; then
    echo "Backup successful: $BACKUP_NAME"
    
    # Prune old backups (keep only the most recent MAX_BACKUPS)
    BACKUP_COUNT=$(ls -1 "$BACKUP_DIR"/patentmine_backup_*.tar.gz 2>/dev/null | wc -l)
    if [ "$BACKUP_COUNT" -gt "$MAX_BACKUPS" ]; then
        echo "Pruning old backups..."
        ls -1tr "$BACKUP_DIR"/patentmine_backup_*.tar.gz | head -n -"$MAX_BACKUPS" | xargs rm
    fi
else
    echo "Error: Backup failed."
    exit 1
fi
