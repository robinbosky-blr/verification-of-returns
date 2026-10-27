#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
backup_path="${1:?Usage: RETURNS_RESTORE_CONFIRM=restore bash scripts/restore.sh BACKUP_DIRECTORY}"
if [[ "${RETURNS_RESTORE_CONFIRM:-}" != restore ]]; then
 printf 'Restore replaces this instance. Back up first, then set RETURNS_RESTORE_CONFIRM=restore.\n' >&2
 exit 1
fi
test -s "$backup_path/database.dump"
test -s "$backup_path/uploads.tar.gz"
docker compose stop api worker
docker compose exec -T db pg_restore -U returns -d returns --clean --if-exists < "$backup_path/database.dump"
docker compose run --rm -T --entrypoint sh api -c 'tar xzf - -C /data' < "$backup_path/uploads.tar.gz"
docker compose up -d api worker web
printf 'Restore completed. Verify case counts before reopening business operations.\n'
