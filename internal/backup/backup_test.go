package backup

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/davemorin/supacrawl/internal/postgres"
	"github.com/davemorin/supacrawl/internal/store"
	"github.com/stretchr/testify/require"
)

func TestEncryptedBackupRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "supacrawl.db"))
	require.NoError(t, err)
	defer st.Close()

	require.NoError(t, st.BeginDataCopy(ctx, false))
	require.NoError(t, st.PutDataRows(ctx, []postgres.TableRow{
		{Schema: "public", TableName: "companies", RowNumber: 1, JSON: `{"name":"Offline Nexus"}`},
	}, false))
	require.NoError(t, st.FinishDataCopy(ctx, postgres.DataCopyStats{Tables: 1, Rows: 1}, nil))

	identity, err := GenerateIdentity()
	require.NoError(t, err)

	repoPath := filepath.Join(dir, "backup")
	writeResult, err := Writer{RepoPath: repoPath, Recipient: identity.Recipient().String()}.Write(ctx, st)
	require.NoError(t, err)
	require.Equal(t, len(store.ArchiveTableNames), writeResult.Shards)
	require.GreaterOrEqual(t, writeResult.Rows, int64(1))

	manifest, manifestPath, err := ReadManifest(repoPath)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repoPath, ManifestName), manifestPath)
	require.Len(t, manifest.Shards, len(store.ArchiveTableNames))

	outDir := filepath.Join(dir, "restore")
	pullResult, err := Puller{RepoPath: repoPath, Identity: identity.String()}.Pull(ctx, outDir)
	require.NoError(t, err)
	require.Equal(t, writeResult.Shards, pullResult.Shards)
	require.Equal(t, writeResult.Rows, pullResult.Rows)

	tableRows, err := os.Open(filepath.Join(outDir, "table_rows.jsonl.gz"))
	require.NoError(t, err)
	defer tableRows.Close()
	reader, err := gzip.NewReader(tableRows)
	require.NoError(t, err)
	defer reader.Close()
	restored, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Contains(t, string(restored), "Offline Nexus")

	encrypted, err := os.ReadFile(filepath.Join(repoPath, "shards", "table_rows.jsonl.gz.age"))
	require.NoError(t, err)
	require.NotContains(t, string(encrypted), "Offline Nexus")
}

func TestPullRejectsInvalidManifestShards(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		shard Shard
	}{
		{
			name: "unsupported table",
			shard: Shard{
				Table:       "sqlite_master",
				File:        "shards/sqlite_master.jsonl.gz.age",
				PlainSHA256: strings.Repeat("0", 64),
			},
		},
		{
			name: "absolute file",
			shard: Shard{
				Table:       "table_rows",
				File:        "/tmp/table_rows.jsonl.gz.age",
				PlainSHA256: strings.Repeat("0", 64),
			},
		},
		{
			name: "traversal file",
			shard: Shard{
				Table:       "table_rows",
				File:        "shards/../table_rows.jsonl.gz.age",
				PlainSHA256: strings.Repeat("0", 64),
			},
		},
		{
			name: "unexpected file",
			shard: Shard{
				Table:       "table_rows",
				File:        "shards/not_table_rows.jsonl.gz.age",
				PlainSHA256: strings.Repeat("0", 64),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			repoPath := filepath.Join(dir, "backup")
			require.NoError(t, os.MkdirAll(filepath.Join(repoPath, "shards"), 0o755))
			writeManifest(t, repoPath, Manifest{Version: 1, Shards: []Shard{tc.shard}})

			identity, err := GenerateIdentity()
			require.NoError(t, err)
			outDir := filepath.Join(dir, "restore")
			_, err = Puller{RepoPath: repoPath, Identity: identity.String()}.Pull(ctx, outDir)
			require.Error(t, err)
			require.NoFileExists(t, filepath.Join(outDir, "table_rows.jsonl.gz"))
		})
	}
}

func TestPullVerifiesPlainSHA256AndDoesNotLeaveCorruptOutput(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "supacrawl.db"))
	require.NoError(t, err)
	defer st.Close()

	identity, err := GenerateIdentity()
	require.NoError(t, err)

	repoPath := filepath.Join(dir, "backup")
	_, err = Writer{RepoPath: repoPath, Recipient: identity.Recipient().String()}.Write(ctx, st)
	require.NoError(t, err)

	manifest, _, err := ReadManifest(repoPath)
	require.NoError(t, err)
	manifest.Shards = []Shard{manifest.Shards[len(manifest.Shards)-1]}
	manifest.Shards[0].PlainSHA256 = strings.Repeat("f", 64)
	writeManifest(t, repoPath, manifest)

	outDir := filepath.Join(dir, "restore")
	_, err = Puller{RepoPath: repoPath, Identity: identity.String()}.Pull(ctx, outDir)
	require.Error(t, err)
	require.ErrorContains(t, err, "plain_sha256")
	require.NoFileExists(t, filepath.Join(outDir, manifest.Shards[0].Table+".jsonl.gz"))
}

func TestPullValidatesReadableGzipAndDoesNotLeaveCorruptOutput(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "backup")
	shardDir := filepath.Join(repoPath, "shards")
	require.NoError(t, os.MkdirAll(shardDir, 0o755))

	identity, err := GenerateIdentity()
	require.NoError(t, err)
	shardPath := filepath.Join(shardDir, "table_rows.jsonl.gz.age")
	out, err := os.Create(shardPath)
	require.NoError(t, err)
	encrypted, err := age.Encrypt(out, identity.Recipient())
	require.NoError(t, err)
	_, err = encrypted.Write([]byte("not a gzip stream"))
	require.NoError(t, err)
	require.NoError(t, encrypted.Close())
	require.NoError(t, out.Close())

	writeManifest(t, repoPath, Manifest{Version: 1, Shards: []Shard{{
		Table:       "table_rows",
		File:        "shards/table_rows.jsonl.gz.age",
		PlainSHA256: strings.Repeat("0", 64),
	}}})

	outDir := filepath.Join(dir, "restore")
	_, err = Puller{RepoPath: repoPath, Identity: identity.String()}.Pull(ctx, outDir)
	require.Error(t, err)
	require.NoFileExists(t, filepath.Join(outDir, "table_rows.jsonl.gz"))
}

func writeManifest(t *testing.T, repoPath string, manifest Manifest) {
	t.Helper()
	data, err := json.MarshalIndent(manifest, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, ManifestName), append(data, '\n'), 0o644))
}
