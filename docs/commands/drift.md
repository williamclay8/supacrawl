# drift

`drift` compares the current archive to a baseline archive, then adds audit
findings and branch inventory from the latest Management API snapshot.

```bash
supacrawl drift ~/.supacrawl/supacrawl-before.db --json
supacrawl drift ./baseline.db --sync never
```

The diff covers tables, columns, indexes, constraints, policies, functions,
triggers, extensions, Storage buckets, and copied table row counts.

Branch inventory is available after:

```bash
SUPABASE_ACCESS_TOKEN=sbp_... supacrawl management sync --project-ref <ref>
```

Without a Management API snapshot, `drift` still returns the archive diff and
audit report, with a warning that branch inventory is unavailable.
