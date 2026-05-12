package store

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/davemorin/supacrawl/internal/postgres"
	"github.com/stretchr/testify/require"
)

func TestPutSnapshotStatusAndSearch(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "supacrawl.db"))
	require.NoError(t, err)
	defer st.Close()

	snapshot := postgres.Snapshot{
		Project: postgres.ProjectInfo{
			ID:            "demo",
			DatabaseName:  "postgres",
			CurrentUser:   "postgres",
			ServerVersion: "16.0",
			CollectedAt:   time.Now().UTC(),
		},
		Schemas: []postgres.Schema{{Name: "public", Owner: "postgres", Type: "user"}},
		Tables:  []postgres.Table{{Schema: "public", Name: "profiles", Kind: "table", Owner: "postgres", Comment: "User profiles", RLSEnabled: true, EstimatedRows: 42}},
		Columns: []postgres.Column{
			{TableSchema: "public", TableName: "profiles", Name: "id", Ordinal: 1, DataType: "uuid", IsNullable: false},
			{TableSchema: "public", TableName: "profiles", Name: "display_name", Ordinal: 2, DataType: "text", IsNullable: true},
		},
		Policies: []postgres.Policy{{Schema: "public", TableName: "profiles", Name: "profiles_select", Command: "SELECT", Roles: "{authenticated}", Using: "auth.uid() = id"}},
	}

	require.NoError(t, st.PutSnapshot(ctx, snapshot))
	status, err := st.Status(ctx)
	require.NoError(t, err)
	require.Equal(t, "demo", status.ProjectID)
	require.Equal(t, 1, status.Tables)
	require.Equal(t, 2, status.Columns)
	require.Equal(t, 1, status.Policies)

	results, err := st.Search(ctx, "profiles authenticated", "", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "table", results[0].Kind)
}

func TestAuditReportsRiskSignals(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "supacrawl.db"))
	require.NoError(t, err)
	defer st.Close()

	require.NoError(t, st.PutSnapshot(ctx, postgres.Snapshot{
		Project: postgres.ProjectInfo{
			ID:            "demo",
			DatabaseName:  "postgres",
			CurrentUser:   "postgres",
			ServerVersion: "16.0",
			CollectedAt:   time.Now().UTC(),
		},
		Tables: []postgres.Table{
			{Schema: "public", Name: "open_profiles", Kind: "table", Owner: "postgres", RLSEnabled: false},
			{Schema: "public", Name: "guarded_profiles", Kind: "table", Owner: "postgres", RLSEnabled: true},
		},
		Functions:      []postgres.Function{{Schema: "public", Name: "admin_touch", Language: "plpgsql", SecurityDefiner: true}},
		StorageBuckets: []postgres.StorageBucket{{ID: "public-assets", Name: "public-assets", Public: true}},
		StorageObjectStats: []postgres.StorageObjectStat{
			{BucketID: "public-assets", ObjectCount: 2, TotalBytes: 512},
		},
	}))
	require.NoError(t, st.BeginDataCopy(ctx, false))
	require.NoError(t, st.PutDataRows(ctx, []postgres.TableRow{
		{Schema: "public", TableName: "open_profiles", RowNumber: 1, JSON: `{"id":1}`},
	}, false))
	require.NoError(t, st.FinishDataCopy(ctx, postgres.DataCopyStats{Tables: 1, Rows: 1}, nil))

	audit, err := st.Audit(ctx, 10)
	require.NoError(t, err)

	require.Equal(t, "demo", audit.Status.ProjectID)
	require.Equal(t, []RiskTable{{Schema: "public", Table: "open_profiles", Kind: "table"}}, audit.TablesWithoutRLS)
	require.Equal(t, []RiskTable{{Schema: "public", Table: "guarded_profiles", Kind: "table"}}, audit.RLSTablesWithoutPolicies)
	require.Equal(t, []RiskStorageBucket{{ID: "public-assets", Name: "public-assets"}}, audit.PublicStorageBuckets)
	require.Equal(t, []RiskFunction{{Schema: "public", Name: "admin_touch", IdentityArgs: "", Language: "plpgsql"}}, audit.SecurityDefinerFunctions)
	require.EqualValues(t, 2, audit.StorageObjectCount)
	require.NotEmpty(t, audit.Warnings)
	require.Contains(t, audit.Warnings[0], "Storage object bytes")
}

func TestManagementSnapshotRoundTrip(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "supacrawl.db"))
	require.NoError(t, err)
	defer st.Close()

	raw := []byte(`{"project_ref":"abcdefghijklmnopqrst","branches":{"available":true,"raw":[{"name":"main"}]}}`)
	saved, err := st.PutManagementSnapshot(ctx, "management-api", "abcdefghijklmnopqrst", raw)
	require.NoError(t, err)
	require.Equal(t, "abcdefghijklmnopqrst", saved.ProjectRef)
	require.NotZero(t, saved.ID)

	latest, err := st.LatestManagementSnapshot(ctx)
	require.NoError(t, err)
	require.Equal(t, saved.ID, latest.ID)
	require.JSONEq(t, string(raw), string(latest.RawJSON))
}

func TestReadOnlyQueryRejectsWrites(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "supacrawl.db"))
	require.NoError(t, err)
	defer st.Close()

	_, err = st.QueryReadOnly(context.Background(), "delete from tables")
	require.Error(t, err)
	_, err = st.QueryReadOnly(context.Background(), "select 1; delete from tables")
	require.Error(t, err)
}

func TestReadOnlyQueryAllowsSafePragmas(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "supacrawl.db"))
	require.NoError(t, err)
	defer st.Close()

	for _, query := range []string{
		"pragma table_info(tables)",
		"pragma table_list",
		"pragma index_list(tables)",
		"pragma database_list",
	} {
		t.Run(query, func(t *testing.T) {
			_, err := st.QueryReadOnly(ctx, query)
			require.NoError(t, err)
		})
	}
}

func TestReadOnlyQueryRejectsMutatingPragmas(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "supacrawl.db"))
	require.NoError(t, err)
	defer st.Close()

	for _, query := range []string{
		"pragma user_version=1",
		"pragma journal_mode=off",
		"pragma wal_checkpoint",
		"pragma writable_schema=ON",
	} {
		t.Run(query, func(t *testing.T) {
			_, err := st.QueryReadOnly(ctx, query)
			require.Error(t, err)
		})
	}
}

func TestIsReadOnlyQueryPreservesSelectWithAndSemicolonRules(t *testing.T) {
	require.True(t, isReadOnlyQuery("select 1"))
	require.True(t, isReadOnlyQuery("with one as (select 1) select * from one"))
	require.False(t, isReadOnlyQuery("select 1;"))
	require.False(t, isReadOnlyQuery(""))
}

func TestPutDataRowsUpdatesStatusAndSQL(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "supacrawl.db"))
	require.NoError(t, err)
	defer st.Close()

	require.NoError(t, st.BeginDataCopy(ctx, true))
	require.NoError(t, st.PutDataRows(ctx, []postgres.TableRow{
		{Schema: "public", TableName: "companies", RowNumber: 1, JSON: `{"id":"1","name":"Offline"}`},
		{Schema: "public", TableName: "companies", RowNumber: 2, JSON: `{"id":"2","name":"Nexus"}`},
	}, true))
	require.NoError(t, st.FinishDataCopy(ctx, postgres.DataCopyStats{Tables: 1, Rows: 2}, nil))

	status, err := st.Status(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, status.DataTables)
	require.EqualValues(t, 2, status.DataRows)

	result, err := st.QueryReadOnly(ctx, `select json_extract(row_json, '$.name') from table_rows where table_name = 'companies' order by row_number`)
	require.NoError(t, err)
	require.Equal(t, [][]string{{"Offline"}, {"Nexus"}}, result.Rows)

	size, err := st.Size(ctx, filepath.Join(t.TempDir(), "missing.db"), 10)
	require.NoError(t, err)
	require.EqualValues(t, len(`{"id":"1","name":"Offline"}`)+len(`{"id":"2","name":"Nexus"}`), size.RowJSONBytes)

	exported, err := st.ExportRows(ctx, "public", "companies", 1)
	require.NoError(t, err)
	require.Len(t, exported, 1)
	require.Equal(t, `{"id":"1","name":"Offline"}`, exported[0].JSON)
}

func TestListStorageObjects(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "supacrawl.db"))
	require.NoError(t, err)
	defer st.Close()

	require.NoError(t, st.BeginDataCopy(ctx, false))
	require.NoError(t, st.PutDataRows(ctx, []postgres.TableRow{
		{Schema: "storage", TableName: "objects", RowNumber: 1, JSON: `{"bucket_id":"pictures","name":"avatars/a.png","metadata":{"size":123}}`},
		{Schema: "storage", TableName: "objects", RowNumber: 2, JSON: `{"bucket_id":"private","name":"docs/b.pdf","metadata":{"size":"456"}}`},
	}, false))

	objects, err := st.ListStorageObjects(ctx, "pictures", 10)
	require.NoError(t, err)
	require.Equal(t, []StorageObject{{BucketID: "pictures", Name: "avatars/a.png", Size: 123}}, objects)
}

func TestPutDataRowsCanSkipFTS(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "supacrawl.db"))
	require.NoError(t, err)
	defer st.Close()

	require.NoError(t, st.BeginDataCopy(ctx, false))
	require.NoError(t, st.PutDataRows(ctx, []postgres.TableRow{
		{Schema: "public", TableName: "companies", RowNumber: 1, JSON: `{"name":"No FTS"}`},
	}, false))

	result, err := st.QueryReadOnly(ctx, `select count(*) from table_row_fts`)
	require.NoError(t, err)
	require.Equal(t, [][]string{{"0"}}, result.Rows)
}

func TestWriteTableJSONL(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "supacrawl.db"))
	require.NoError(t, err)
	defer st.Close()

	require.NoError(t, st.BeginDataCopy(ctx, false))
	require.NoError(t, st.PutDataRows(ctx, []postgres.TableRow{
		{Schema: "public", TableName: "companies", RowNumber: 1, JSON: `{"name":"Nexus"}`},
	}, false))

	var out bytes.Buffer
	rows, err := st.WriteTableJSONL(ctx, "table_rows", &out)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)
	require.Contains(t, out.String(), `"table_name":"companies"`)

	_, err = st.WriteTableJSONL(ctx, "not_allowed", &out)
	require.Error(t, err)
}
