# audit

`audit` reads the local archive and prints high-signal warnings for Supabase
review and backup readiness.

```bash
supacrawl audit --json
supacrawl audit --limit 25 --sync never
```

It reports:

- user-schema tables without RLS
- RLS-enabled tables without policies
- public Storage buckets
- security-definer functions
- largest copied tables from `table_rows`
- warnings when Storage objects, copied data, or Management API snapshots need
  separate handling

`audit` is local-first. It follows the same `--sync auto|always|never` and
`--stale-after` read-refresh flags as `status`.
