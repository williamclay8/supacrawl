package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/davemorin/supacrawl/internal/postgres"
	"github.com/stretchr/testify/require"
)

func TestDiff_AddedTable(t *testing.T) {
	result := diffSnapshots(t,
		snapshotWith(t, "demo", []postgres.Table{profilesTable()}, nil, nil),
		snapshotWith(t, "demo", []postgres.Table{profilesTable(), auditLogTable()}, nil, nil),
	)

	require.Len(t, result.Tables.Added, 1)
	require.Equal(t, "audit_log", result.Tables.Added[0].Name)
	require.Empty(t, result.Tables.Removed)
	require.Empty(t, result.Tables.Changed)
}

func TestDiff_RemovedTable(t *testing.T) {
	result := diffSnapshots(t,
		snapshotWith(t, "demo", []postgres.Table{auditLogTable(), profilesTable()}, nil, nil),
		snapshotWith(t, "demo", []postgres.Table{profilesTable()}, nil, nil),
	)

	require.Len(t, result.Tables.Removed, 1)
	require.Equal(t, "audit_log", result.Tables.Removed[0].Name)
	require.Empty(t, result.Tables.Added)
	require.Empty(t, result.Tables.Changed)
}

func TestDiff_RLSDisabled(t *testing.T) {
	before := profilesTable()
	before.RLSEnabled = true
	after := before
	after.RLSEnabled = false

	result := diffSnapshots(t,
		snapshotWith(t, "demo", []postgres.Table{before}, nil, nil),
		snapshotWith(t, "demo", []postgres.Table{after}, nil, nil),
	)

	require.Len(t, result.Tables.Changed, 1)
	change := result.Tables.Changed[0]
	require.Equal(t, "public.profiles", change.Key)
	require.Equal(t, []string{"rls_enabled"}, change.ChangedFields)
	require.True(t, change.Before.RLSEnabled)
	require.False(t, change.After.RLSEnabled)
}

func TestDiff_PolicyUsingChanged(t *testing.T) {
	before := profilesPolicy()
	before.Using = "auth.uid() = id"
	after := before
	after.Using = "true"

	result := diffSnapshots(t,
		snapshotWith(t, "demo", []postgres.Table{profilesTable()}, []postgres.Policy{before}, nil),
		snapshotWith(t, "demo", []postgres.Table{profilesTable()}, []postgres.Policy{after}, nil),
	)

	require.Len(t, result.Policies.Changed, 1)
	change := result.Policies.Changed[0]
	require.Equal(t, "public.profiles.profiles_select", change.Key)
	require.Equal(t, []string{"using"}, change.ChangedFields)
	require.Equal(t, "auth.uid() = id", change.Before.Using)
	require.Equal(t, "true", change.After.Using)
}

func TestDiff_StorageBucketPublic(t *testing.T) {
	before := avatarsBucket()
	before.Public = false
	after := before
	after.Public = true

	result := diffSnapshots(t,
		snapshotWith(t, "demo", nil, nil, []postgres.StorageBucket{before}),
		snapshotWith(t, "demo", nil, nil, []postgres.StorageBucket{after}),
	)

	require.Len(t, result.StorageBuckets.Changed, 1)
	change := result.StorageBuckets.Changed[0]
	require.Equal(t, "avatars", change.Key)
	require.Equal(t, []string{"public"}, change.ChangedFields)
	require.False(t, change.Before.Public)
	require.True(t, change.After.Public)
}

func TestDiff_Identical(t *testing.T) {
	result := diffSnapshots(t,
		snapshotWith(t, "demo", []postgres.Table{profilesTable()}, []postgres.Policy{profilesPolicy()}, []postgres.StorageBucket{avatarsBucket()}),
		snapshotWith(t, "demo", []postgres.Table{profilesTable()}, []postgres.Policy{profilesPolicy()}, []postgres.StorageBucket{avatarsBucket()}),
	)

	require.NotNil(t, result.Tables.Added)
	require.NotNil(t, result.Tables.Removed)
	require.NotNil(t, result.Tables.Changed)
	require.Empty(t, result.Tables.Added)
	require.Empty(t, result.Tables.Removed)
	require.Empty(t, result.Tables.Changed)
	require.Empty(t, result.Policies.Added)
	require.Empty(t, result.Policies.Removed)
	require.Empty(t, result.Policies.Changed)
	require.Empty(t, result.StorageBuckets.Added)
	require.Empty(t, result.StorageBuckets.Removed)
	require.Empty(t, result.StorageBuckets.Changed)
}

func TestDiff_ExpandedJSONFields(t *testing.T) {
	baseline := snapshotWithExpandedObjects()
	current := snapshotWithExpandedObjects()
	current.Columns[0].Ordinal = 2
	current.Columns[0].DataType = "uuid"
	current.Columns[0].IsNullable = true
	current.Columns[0].Default = "gen_random_uuid()"
	current.Columns[0].Comment = "Primary profile identifier"
	current.Indexes[0].Definition = "create unique index profiles_email_idx on public.profiles(email)"
	current.Constraints[0].Type = "u"
	current.Constraints[0].Definition = "unique (email)"
	current.Functions[0].Returns = "bigint"
	current.Functions[0].Language = "plpgsql"
	current.Functions[0].SecurityDefiner = true
	current.Functions[0].Definition = "begin return 1; end"
	current.Triggers[0].Timing = "BEFORE"
	current.Triggers[0].Events = "INSERT,UPDATE"
	current.Triggers[0].FunctionName = "public.touch_profile_insert()"
	current.Triggers[0].Definition = "create trigger touch_profile before insert or update on public.profiles execute function public.touch_profile()"
	current.Extensions[0].Schema = "extensions"
	current.Extensions[0].Version = "1.4"
	current.Extensions[0].Comment = "cryptographic functions"

	result := diffSnapshots(t, baseline, current)

	require.Len(t, result.Columns.Changed, 1)
	require.Equal(t, "public.profiles.id", result.Columns.Changed[0].Key)
	require.Equal(t, []string{"comment", "data_type", "default", "is_nullable", "ordinal"}, result.Columns.Changed[0].ChangedFields)
	require.Len(t, result.Indexes.Changed, 1)
	require.Equal(t, []string{"definition"}, result.Indexes.Changed[0].ChangedFields)
	require.Len(t, result.Constraints.Changed, 1)
	require.Equal(t, []string{"definition", "type"}, result.Constraints.Changed[0].ChangedFields)
	require.Len(t, result.Functions.Changed, 1)
	require.Equal(t, []string{"definition", "language", "returns", "security_definer"}, result.Functions.Changed[0].ChangedFields)
	require.Len(t, result.Triggers.Changed, 1)
	require.Equal(t, []string{"definition", "events", "function_name", "timing"}, result.Triggers.Changed[0].ChangedFields)
	require.Len(t, result.Extensions.Changed, 1)
	require.Equal(t, []string{"comment", "schema", "version"}, result.Extensions.Changed[0].ChangedFields)

	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &fields))
	for _, field := range []string{"columns", "indexes", "constraints", "functions", "triggers", "extensions", "table_rows"} {
		require.Contains(t, fields, field)
	}
}

func TestDiff_TableRowsCountsByTable(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "baseline.db")
	currentPath := filepath.Join(dir, "current.db")
	writeSnapshot(t, baselinePath, snapshotWith(t, "demo", []postgres.Table{profilesTable()}, nil, nil))
	writeSnapshot(t, currentPath, snapshotWith(t, "demo", []postgres.Table{profilesTable(), auditLogTable()}, nil, nil))
	writeDataRows(t, baselinePath, []postgres.TableRow{
		{Schema: "public", TableName: "orphaned", RowNumber: 1, JSON: `{"id":1}`},
		{Schema: "public", TableName: "profiles", RowNumber: 1, JSON: `{"id":1}`},
	})
	writeDataRows(t, currentPath, []postgres.TableRow{
		{Schema: "public", TableName: "audit_log", RowNumber: 1, JSON: `{"id":1}`},
		{Schema: "public", TableName: "profiles", RowNumber: 1, JSON: `{"id":1}`},
		{Schema: "public", TableName: "profiles", RowNumber: 2, JSON: `{"id":2}`},
		{Schema: "public", TableName: "profiles", RowNumber: 3, JSON: `{"id":3}`},
	})

	current, err := Open(currentPath)
	require.NoError(t, err)
	defer current.Close()
	baseline, err := OpenReadOnly(baselinePath)
	require.NoError(t, err)
	defer baseline.Close()

	result, err := current.Diff(ctx, baseline)
	require.NoError(t, err)

	require.Equal(t, []TableRowCount{{Key: "public.audit_log", Schema: "public", TableName: "audit_log", Rows: 1}}, result.TableRows.Added)
	require.Equal(t, []TableRowCount{{Key: "public.orphaned", Schema: "public", TableName: "orphaned", Rows: 1}}, result.TableRows.Removed)
	require.Equal(t, []TableRowCountChange{{
		Key:    "public.profiles",
		Before: TableRowCount{Key: "public.profiles", Schema: "public", TableName: "profiles", Rows: 1},
		After:  TableRowCount{Key: "public.profiles", Schema: "public", TableName: "profiles", Rows: 3},
	}}, result.TableRows.Changed)
}

func TestDiff_ProjectMismatch(t *testing.T) {
	result := diffSnapshots(t,
		snapshotWith(t, "staging", []postgres.Table{profilesTable()}, nil, nil),
		snapshotWith(t, "demo", []postgres.Table{profilesTable()}, nil, nil),
	)

	require.Equal(t, "demo", result.Current.ProjectID)
	require.Equal(t, "staging", result.Baseline.ProjectID)
	require.True(t, result.ProjectMismatch)
}

func TestOpenReadOnly_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")

	_, err := OpenReadOnly(path)

	require.Error(t, err)
	require.Contains(t, err.Error(), "baseline archive not found: "+path)
	_, statErr := os.Stat(path)
	require.True(t, os.IsNotExist(statErr))
}

func TestOpenReadOnly_RejectsNonArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.db")
	require.NoError(t, os.WriteFile(path, nil, 0o600))

	st, err := OpenReadOnly(path)

	require.Nil(t, st)
	require.Error(t, err)
	require.Contains(t, err.Error(), "path is not a supacrawl archive: "+path)
}

func TestOpenReadOnly_AcceptsInitializedArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive.db")
	writable, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, writable.Close())

	st, err := OpenReadOnly(path)
	require.NoError(t, err)
	defer st.Close()

	result, err := st.QueryReadOnly(context.Background(), "select count(*) as tables from tables")
	require.NoError(t, err)
	require.Equal(t, [][]string{{"0"}}, result.Rows)
}

func diffSnapshots(t *testing.T, baselineSnapshot, currentSnapshot postgres.Snapshot) DiffResult {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "baseline.db")
	currentPath := filepath.Join(dir, "current.db")
	writeSnapshot(t, baselinePath, baselineSnapshot)
	writeSnapshot(t, currentPath, currentSnapshot)

	current, err := Open(currentPath)
	require.NoError(t, err)
	defer current.Close()
	baseline, err := OpenReadOnly(baselinePath)
	require.NoError(t, err)
	defer baseline.Close()

	result, err := current.Diff(ctx, baseline)
	require.NoError(t, err)
	return result
}

func writeSnapshot(t *testing.T, path string, snapshot postgres.Snapshot) {
	t.Helper()
	st, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, st.PutSnapshot(context.Background(), snapshot))
	require.NoError(t, st.Close())
}

func writeDataRows(t *testing.T, path string, rows []postgres.TableRow) {
	t.Helper()
	st, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, st.BeginDataCopy(context.Background(), false))
	require.NoError(t, st.PutDataRows(context.Background(), rows, false))
	require.NoError(t, st.Close())
}

func snapshotWith(t *testing.T, projectID string, tables []postgres.Table, policies []postgres.Policy, buckets []postgres.StorageBucket) postgres.Snapshot {
	t.Helper()
	return postgres.Snapshot{
		Project: postgres.ProjectInfo{
			ID:            projectID,
			DatabaseName:  "postgres",
			CurrentUser:   "postgres",
			ServerVersion: "16.0",
			CollectedAt:   time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
		},
		Tables:         tables,
		Policies:       policies,
		StorageBuckets: buckets,
	}
}

func snapshotWithExpandedObjects() postgres.Snapshot {
	snapshot := postgres.Snapshot{
		Project: postgres.ProjectInfo{
			ID:            "demo",
			DatabaseName:  "postgres",
			CurrentUser:   "postgres",
			ServerVersion: "16.0",
			CollectedAt:   time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
		},
		Tables: []postgres.Table{profilesTable()},
		Columns: []postgres.Column{{
			TableSchema: "public",
			TableName:   "profiles",
			Name:        "id",
			Ordinal:     1,
			DataType:    "integer",
			IsNullable:  false,
			Default:     "",
			Comment:     "",
		}},
		Indexes: []postgres.Index{{
			Schema:     "public",
			TableName:  "profiles",
			Name:       "profiles_pkey",
			Definition: "create unique index profiles_pkey on public.profiles(id)",
		}},
		Constraints: []postgres.Constraint{{
			Schema:     "public",
			TableName:  "profiles",
			Name:       "profiles_pkey",
			Type:       "p",
			Definition: "primary key (id)",
		}},
		Functions: []postgres.Function{{
			Schema:          "public",
			Name:            "profile_count",
			IdentityArgs:    "",
			Returns:         "integer",
			Language:        "sql",
			SecurityDefiner: false,
			Definition:      "select 1",
		}},
		Triggers: []postgres.Trigger{{
			Schema:       "public",
			TableName:    "profiles",
			Name:         "touch_profile",
			Timing:       "AFTER",
			Events:       "UPDATE",
			FunctionName: "public.touch_profile()",
			Definition:   "create trigger touch_profile after update on public.profiles execute function public.touch_profile()",
		}},
		Extensions: []postgres.Extension{{
			Name:    "pgcrypto",
			Schema:  "public",
			Version: "1.3",
			Comment: "",
		}},
	}
	return snapshot
}

func profilesTable() postgres.Table {
	return postgres.Table{
		Schema:        "public",
		Name:          "profiles",
		Kind:          "table",
		Owner:         "postgres",
		Comment:       "User profiles",
		RLSEnabled:    true,
		RLSForced:     false,
		EstimatedRows: 42,
	}
}

func auditLogTable() postgres.Table {
	return postgres.Table{
		Schema:        "public",
		Name:          "audit_log",
		Kind:          "table",
		Owner:         "postgres",
		RLSEnabled:    true,
		RLSForced:     true,
		EstimatedRows: 10,
	}
}

func profilesPolicy() postgres.Policy {
	return postgres.Policy{
		Schema:    "public",
		TableName: "profiles",
		Name:      "profiles_select",
		Command:   "SELECT",
		Roles:     "{authenticated}",
		Using:     "auth.uid() = id",
		Check:     "",
	}
}

func avatarsBucket() postgres.StorageBucket {
	return postgres.StorageBucket{
		ID:               "avatars",
		Name:             "avatars",
		Public:           false,
		FileSizeLimit:    "1048576",
		AllowedMimeTypes: "{image/png,image/jpeg}",
		CreatedAt:        "2026-05-10T12:00:00Z",
		UpdatedAt:        "2026-05-10T12:00:00Z",
	}
}
