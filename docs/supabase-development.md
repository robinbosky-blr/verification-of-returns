# Supabase development environment

Supabase is an optional development and acceptance-testing platform for ReturnScope. It supplies hosted PostgreSQL and private original-file storage. The Go API and worker still own parsing, reconciliation, claim rules, financial calculations, authorization and exports.

Amazon, Flipkart and Myntra remain separated by their existing database schemas, marketplace keys, parsers and rule versions. Supabase does not merge or replace those engines.

## 1. Create the Supabase resources

1. Create a separate Supabase project for development. Do not use the future production project for parser experiments.
2. In **Storage**, create a private bucket named `returns-originals`.
3. In **Project Settings > API Keys**, create or copy a secret key for the trusted Go backend. Never use this key in Svelte, browser code, screenshots, Git, or support messages.
4. Open **Connect** and copy the Session pooler connection string. Session mode works from IPv4 development networks and supports the persistent Go API and worker.

## 2. Configure the application

```sh
cp .env.supabase.example .env.supabase
```

On Windows PowerShell:

```powershell
Copy-Item .env.supabase.example .env.supabase
```

Replace the placeholders in `.env.supabase`. Keep `sslmode=require` in `DATABASE_URL`. If the database password contains reserved URL characters, percent-encode them.

The secret key stays only in the API and worker containers. The frontend continues to call `/api` and never connects directly to privileged Supabase services.

## 3. Start the development application

```sh
docker compose --env-file .env.supabase -f compose.supabase.yaml up -d --build
```

Open `http://localhost:8088`. The API applies the committed migrations to the Supabase Postgres database at startup. Upload a small, reviewed file for each marketplace before testing large files.

Check the services:

```sh
docker compose --env-file .env.supabase -f compose.supabase.yaml ps
docker compose --env-file .env.supabase -f compose.supabase.yaml logs -f api worker
```

Stop the application without deleting hosted data:

```sh
docker compose --env-file .env.supabase -f compose.supabase.yaml down
```

## 4. Acceptance checks

- Upload Website, Physical and Payment files separately for Amazon, Flipkart and Myntra.
- Confirm each marketplace shows only its own imports, cases, claims and audit records.
- Delete an incorrect import and confirm its derived rows are recalculated while manual claim history remains.
- Confirm original files appear in the private `returns-originals` bucket.
- Confirm claim dates and financial calculations against the reviewed source workbooks.
- Run `go test -race ./...`, `go vet ./...`, `npm run check` and `npm run build` before merging.

## Security and backup limits

Supabase secret keys bypass Row Level Security and must remain server-side. The private bucket is not a database backup. Supabase Storage S3 compatibility does not provide object versioning, so keep separate database and original-file backups before production use.
