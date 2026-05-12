package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/davemorin/supacrawl/internal/postgres"
	"github.com/davemorin/supacrawl/internal/store"
	"github.com/stretchr/testify/require"
)

func TestInitAndStatus(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	dbPath := filepath.Join(dir, "supacrawl.db")
	var stdout bytes.Buffer
	app := &App{Stdout: &stdout, Stderr: &bytes.Buffer{}}

	require.NoError(t, app.Run(context.Background(), []string{"--config", configPath, "init", "--db", dbPath, "--project-id", "demo"}))
	require.Contains(t, stdout.String(), "Init")
	stdout.Reset()

	require.NoError(t, app.Run(context.Background(), []string{"--config", configPath, "status"}))
	require.Contains(t, stdout.String(), "Status")
	require.Contains(t, stdout.String(), "schemas: 0")
}

func TestGlobalJSONFlagWorksAfterCommand(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	dbPath := filepath.Join(dir, "supacrawl.db")
	var stdout bytes.Buffer
	app := &App{Stdout: &stdout, Stderr: &bytes.Buffer{}}

	require.NoError(t, app.Run(context.Background(), []string{"--config", configPath, "init", "--db", dbPath}))
	stdout.Reset()

	require.NoError(t, app.Run(context.Background(), []string{"status", "--config", configPath, "--json"}))
	require.Contains(t, stdout.String(), `"schemas": 0`)
}

func TestParseSearchArgsAcceptsFlagsAfterQuery(t *testing.T) {
	kind, limit, query, err := parseSearchArgs([]string{"measurement", "--limit", "8", "--kind=table"})
	require.NoError(t, err)
	require.Equal(t, "table", kind)
	require.Equal(t, 8, limit)
	require.Equal(t, "measurement", query)
}

func TestParseReadSyncArgs(t *testing.T) {
	args, overrides, err := parseReadSyncArgs([]string{"--sync=never", "profiles", "--stale-after", "1h", "--limit", "5"})
	require.NoError(t, err)
	require.Equal(t, []string{"profiles", "--limit", "5"}, args)
	require.True(t, overrides.PolicySet)
	require.True(t, overrides.StaleAfterSet)
	require.Equal(t, "never", overrides.Policy)
	require.Equal(t, "1h", overrides.StaleAfter)
}

func TestMetadataCommand(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{Stdout: &stdout, Stderr: &bytes.Buffer{}}

	require.NoError(t, app.Run(context.Background(), []string{"metadata", "--json"}))
	require.Contains(t, stdout.String(), `"name": "supacrawl"`)
	require.Contains(t, stdout.String(), `"row-copy"`)
	require.Contains(t, stdout.String(), `"encrypted-backup"`)
}

func TestDiffCommandJSON(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	currentPath := filepath.Join(dir, "supacrawl.db")
	baselinePath := filepath.Join(dir, "baseline.db")
	var stdout bytes.Buffer
	app := &App{Stdout: &stdout, Stderr: &bytes.Buffer{}}

	require.NoError(t, app.Run(ctx, []string{"--config", configPath, "init", "--db", currentPath, "--project-id", "demo"}))
	stdout.Reset()
	writeTestSnapshot(t, baselinePath, true)
	writeTestSnapshot(t, currentPath, false)

	require.NoError(t, app.Run(ctx, []string{"--config", configPath, "diff", baselinePath, "--json", "--sync", "never"}))
	require.Contains(t, stdout.String(), `"project_mismatch": false`)
	require.Contains(t, stdout.String(), `"changed_fields": [`)
	require.Contains(t, stdout.String(), `"rls_enabled"`)
}

func TestAuditCommandJSON(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	dbPath := filepath.Join(dir, "supacrawl.db")
	var stdout bytes.Buffer
	app := &App{Stdout: &stdout, Stderr: &bytes.Buffer{}}

	require.NoError(t, app.Run(ctx, []string{"--config", configPath, "init", "--db", dbPath, "--project-id", "demo"}))
	stdout.Reset()
	st, err := store.Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, st.PutSnapshot(ctx, postgres.Snapshot{
		Project: postgres.ProjectInfo{ID: "demo", DatabaseName: "postgres", CurrentUser: "postgres", ServerVersion: "16.0", CollectedAt: time.Now().UTC()},
		Tables:  []postgres.Table{{Schema: "public", Name: "profiles", Kind: "table", Owner: "postgres", RLSEnabled: false}},
	}))
	require.NoError(t, st.Close())

	require.NoError(t, app.Run(ctx, []string{"--config", configPath, "audit", "--json", "--sync", "never"}))
	require.Contains(t, stdout.String(), `"tables_without_rls"`)
	require.Contains(t, stdout.String(), `"profiles"`)
}

func TestContextCommandJSON(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	dbPath := filepath.Join(dir, "supacrawl.db")
	var stdout bytes.Buffer
	app := &App{Stdout: &stdout, Stderr: &bytes.Buffer{}}

	require.NoError(t, app.Run(ctx, []string{"--config", configPath, "init", "--db", dbPath, "--project-id", "demo"}))
	stdout.Reset()

	require.NoError(t, app.Run(ctx, []string{"--config", configPath, "context", "--json", "--sync", "never"}))
	require.Contains(t, stdout.String(), `"status"`)
	require.Contains(t, stdout.String(), `"audit"`)
	require.Contains(t, stdout.String(), `"management"`)
}

func TestManagementSyncStoresSnapshot(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	dbPath := filepath.Join(dir, "supacrawl.db")
	var stdout bytes.Buffer
	app := &App{Stdout: &stdout, Stderr: &bytes.Buffer{}}
	t.Setenv("SUPABASE_ACCESS_TOKEN_TEST", "test-token")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/projects/abcdefghijklmnopqrst/functions":
			_, _ = w.Write([]byte(`[{"slug":"hello"}]`))
		case "/v1/projects/abcdefghijklmnopqrst/branches":
			_, _ = w.Write([]byte(`[{"name":"main"}]`))
		case "/v1/projects/abcdefghijklmnopqrst/secrets":
			_, _ = w.Write([]byte(`[{"name":"API_TOKEN","value":"must-not-persist"}]`))
		case "/v1/projects/abcdefghijklmnopqrst/config/auth":
			_, _ = w.Write([]byte(`{"site_url":"https://example.test"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	require.NoError(t, app.Run(ctx, []string{"--config", configPath, "init", "--db", dbPath}))
	stdout.Reset()
	require.NoError(t, app.Run(ctx, []string{
		"--config", configPath,
		"management", "sync",
		"--project-ref", "abcdefghijklmnopqrst",
		"--api-url", server.URL,
		"--token-env", "SUPABASE_ACCESS_TOKEN_TEST",
		"--json",
	}))
	require.Contains(t, stdout.String(), `"project_ref": "abcdefghijklmnopqrst"`)
	require.NotContains(t, stdout.String(), "must-not-persist")

	st, err := store.Open(dbPath)
	require.NoError(t, err)
	defer st.Close()
	latest, err := st.LatestManagementSnapshot(ctx)
	require.NoError(t, err)
	require.Equal(t, "abcdefghijklmnopqrst", latest.ProjectRef)
	require.NotContains(t, string(latest.RawJSON), "must-not-persist")
}

func TestDriftCommandIncludesArchiveAuditAndBranchInventory(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	currentPath := filepath.Join(dir, "current.db")
	baselinePath := filepath.Join(dir, "baseline.db")
	var stdout bytes.Buffer
	app := &App{Stdout: &stdout, Stderr: &bytes.Buffer{}}

	require.NoError(t, app.Run(ctx, []string{"--config", configPath, "init", "--db", currentPath, "--project-id", "demo"}))
	writeTestSnapshot(t, baselinePath, true)
	writeTestSnapshot(t, currentPath, false)
	st, err := store.Open(currentPath)
	require.NoError(t, err)
	_, err = st.PutManagementSnapshot(ctx, "management-api", "abcdefghijklmnopqrst", []byte(`{
		"project_ref":"abcdefghijklmnopqrst",
		"branches":{"available":true,"raw":[{"name":"main"},{"name":"preview"}]}
	}`))
	require.NoError(t, err)
	require.NoError(t, st.Close())
	stdout.Reset()

	require.NoError(t, app.Run(ctx, []string{"--config", configPath, "drift", baselinePath, "--json", "--sync", "never"}))
	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Contains(t, payload, "diff")
	require.Contains(t, payload, "audit")
	require.Contains(t, payload, "branches")
}

func TestDataProgressWritesToStderr(t *testing.T) {
	var stderr bytes.Buffer
	app := &App{Stdout: &bytes.Buffer{}, Stderr: &stderr}
	progress := app.dataProgress(FormatText, false)
	require.NotNil(t, progress)

	progress(postgres.DataCopyProgress{Schema: "public", TableName: "companies"})
	progress(postgres.DataCopyProgress{Schema: "public", TableName: "companies", Rows: 10000})
	progress(postgres.DataCopyProgress{Schema: "public", TableName: "companies", Rows: 2, Done: true})

	require.Contains(t, stderr.String(), "copying public.companies")
	require.Contains(t, stderr.String(), "copying public.companies: 10000 rows")
	require.Contains(t, stderr.String(), "copied public.companies: 2 rows")
}

func writeTestSnapshot(t *testing.T, path string, rlsEnabled bool) {
	t.Helper()
	st, err := store.Open(path)
	require.NoError(t, err)
	defer st.Close()

	require.NoError(t, st.PutSnapshot(context.Background(), postgres.Snapshot{
		Project: postgres.ProjectInfo{
			ID:            "demo",
			DatabaseName:  "postgres",
			CurrentUser:   "postgres",
			ServerVersion: "16.0",
			CollectedAt:   time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
		},
		Tables: []postgres.Table{{
			Schema:     "public",
			Name:       "profiles",
			Kind:       "table",
			Owner:      "postgres",
			RLSEnabled: rlsEnabled,
		}},
	}))
}

func TestStringListFlagCollectsRepeatedValues(t *testing.T) {
	var values stringListFlag

	require.NoError(t, values.Set("public.enrichments"))
	require.NoError(t, values.Set("auth.audit_log_entries"))

	require.Equal(t, stringListFlag{"public.enrichments", "auth.audit_log_entries"}, values)
	require.Equal(t, "public.enrichments,auth.audit_log_entries", values.String())
	require.Error(t, values.Set(" "))
}
