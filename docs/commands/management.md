# management

`management` stores read-only Supabase Management API metadata in the local
archive.

```bash
export SUPABASE_ACCESS_TOKEN="sbp_..."
supacrawl management sync --project-ref <ref> --json
supacrawl management status --json
```

`management sync` crawls project sections including functions, branches, auth
config, PostgREST config, Storage config, Realtime config, backups, advisor
results, and secrets metadata. Secret values are scrubbed before the snapshot is
written to `management_snapshots`.

Useful flags:

- `--project-ref <ref>` selects the Supabase project.
- `--token-env SUPABASE_ACCESS_TOKEN` selects the token environment variable.
- `--api-url <url>` points at a test or alternate Management API endpoint.

Supabase `404` responses for optional sections are recorded as unavailable.
Authentication, rate-limit, and other non-success responses fail the command.
