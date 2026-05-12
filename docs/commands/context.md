# context

`context` emits a compact handoff packet for humans, agents, and CI systems.

```bash
supacrawl context --json
supacrawl context --limit 25 --sync never
```

The packet includes:

- tool and version
- archive status counts
- schema/policy report
- audit findings
- latest Management API snapshot and branch inventory when available

Use it before handing a Supabase archive to another workflow so the receiver can
reason from one stable JSON surface instead of running several commands.
