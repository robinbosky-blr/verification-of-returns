# Operations

## Local and server startup

Copy `.env.example` to `.env`, set passwords, and run `docker compose up -d --build`. Open http://localhost:8088. Logs: `docker compose logs -f api worker`. Restart: `docker compose restart`. Stop while preserving data: `docker compose down`.

For a server, keep the default loopback binding and put an HTTPS reverse proxy in front of port 8088. Set `COOKIE_SECURE=true` for HTTPS. Set a DNS name, obtain a certificate, restrict inbound ports to HTTPS/SSH, and keep the database private. This repository does not provision a remote machine or invent hosting credentials.

## Backup

Run `bash scripts/backup.sh`. It writes a PostgreSQL custom-format dump and an archive of original uploads to `backups/<UTC timestamp>/`. Check the command exit status. S3 mode requires a separately configured bucket backup/versioning policy because there may be no originals in the local upload volume.

Schedule a daily backup with your operating system scheduler (cron on Linux, Task Scheduler calling WSL on Windows). Keep at least 7 daily and 4 weekly backups. Copy backups off the machine to encrypted cloud storage or a second system. The included script creates backups; it does not claim to have configured your scheduler or cloud account. Test restore on an isolated Compose project monthly.

Example cron (adjust the absolute repository path):

```cron
0 2 * * * cd /opt/verification-of-returns && /bin/bash scripts/backup.sh >> /var/log/returns-backup.log 2>&1
```

Restore is intentionally documented rather than triggered in the UI. On a new disposable instance, stop API and worker, restore the dump with `pg_restore --clean --if-exists`, restore the upload archive to the uploads volume, then start API and worker and inspect dashboard totals. `scripts/restore.sh BACKUP_DIRECTORY` performs those steps only when `RETURNS_RESTORE_CONFIRM=restore` is set. It overwrites the selected instance; make a backup first.

## Updates

Back up first. Pull the tested source commit, then `docker compose up -d --build`. Preserve volumes. Migrations run under a database lock at startup. Do not downgrade a schema without a compatible migration. PostgreSQL major upgrades require the standard dump/restore or pg_upgrade procedure; never point a new major image at an old data directory blindly.

## Failed files

The worker rejects a whole invalid file, reports worksheet/row context, and leaves prior results in place. Fix and upload the file as a new immutable version. Original files remain available for audit and backup. Excluding a completed import is reversible. A byte-identical failed upload returns the existing failed import, preventing duplicate queue spam.

## Initial acceptance

Upload the real website, physical and corrected payment files. Compare case-level differences with your reviewed workbook, including all missing-payment and ambiguous rows. Never substitute the synthetic fixture count for business acceptance. The January–September source files were unavailable during development.
