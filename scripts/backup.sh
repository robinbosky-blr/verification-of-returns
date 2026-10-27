#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
backup_root="${BACKUP_DIR:-backups}"
backup_path="$backup_root/$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$backup_path"
chmod 700 "$backup_path"
docker compose exec -T db pg_dump -U returns -d returns -Fc > "$backup_path/database.dump"
docker compose exec -T api tar czf - -C /data . > "$backup_path/uploads.tar.gz"
test -s "$backup_path/database.dump"
test -s "$backup_path/uploads.tar.gz"
printf 'Backup completed: %s\n' "$backup_path"
