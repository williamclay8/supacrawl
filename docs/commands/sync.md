# sync

`sync` refreshes the local SQLite archive from Supabase/Postgres.

```bash
supacrawl sync
supacrawl sync --full
supacrawl sync --full --no-row-fts
```

The default mode copies metadata only: schemas, tables, columns, indexes,
constraints, RLS policies, functions, triggers, extensions, storage buckets,
and storage object counts.

`--full` also copies accessible base-table rows into `table_rows` as JSON.
Progress is printed to stderr one table at a time so JSON stdout remains
machine-readable.

Use `--no-row-fts` when you want a smaller archive and do not need full-text
search over row JSON.

Useful flags:

- `--batch-size 2000` controls row insert batch size.
- `--no-progress` disables table progress logs.
- `--project-id <ref>` overrides the archive label for this run.

## Read Command Refresh

Read commands use config `[sync]` settings:

```toml
[sync]
read_policy = "auto"
stale_after = "15m"
```

Supported policy values are:

- `auto`: refresh metadata when stale, but keep reading locally if the database
  URL is unavailable.
- `always`: refresh metadata before the read and return an error if the database
  URL is unavailable.
- `never`: read only from the local SQLite archive.

The same flags work on `status`, `report`, `diff`, `drift`, `audit`,
`context`, `search`, `sql`, `size`, and `export`:

```bash
supacrawl status --sync never
supacrawl report --sync always
supacrawl search --stale-after 1h "profiles"
```
