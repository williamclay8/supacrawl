package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/davemorin/supacrawl/internal/store"
	"github.com/stretchr/testify/require"
)

func TestDownloadObjects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/storage/v1/object/pictures/avatars/a.png", r.URL.Path)
		require.Equal(t, "Bearer service-key", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte("image bytes"))
	}))
	defer server.Close()

	dir := t.TempDir()
	stats, err := (Downloader{BaseURL: server.URL, AuthToken: "service-key"}).DownloadObjects(context.Background(), []store.StorageObject{
		{BucketID: "pictures", Name: "avatars/a.png"},
	}, dir, false)
	require.NoError(t, err)
	require.Equal(t, DownloadStats{Objects: 1, Bytes: int64(len("image bytes")), Skipped: 0}, stats)

	data, err := os.ReadFile(filepath.Join(dir, "pictures", "avatars", "a.png"))
	require.NoError(t, err)
	require.Equal(t, "image bytes", string(data))
}

func TestDownloadObjectsFallsBackToAuthenticatedPath(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/storage/v1/object/pictures/avatars/a.png" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		require.Equal(t, "/storage/v1/object/authenticated/pictures/avatars/a.png", r.URL.Path)
		_, _ = w.Write([]byte("private bytes"))
	}))
	defer server.Close()

	stats, err := (Downloader{BaseURL: server.URL, AuthToken: "service-key"}).DownloadObjects(context.Background(), []store.StorageObject{
		{BucketID: "pictures", Name: "avatars/a.png"},
	}, t.TempDir(), false)
	require.NoError(t, err)
	require.Equal(t, 1, stats.Objects)
	require.Equal(t, []string{"/storage/v1/object/pictures/avatars/a.png", "/storage/v1/object/authenticated/pictures/avatars/a.png"}, paths)
}

func TestObjectPathRejectsTraversal(t *testing.T) {
	_, err := objectPath(t.TempDir(), store.StorageObject{BucketID: "pictures", Name: "../secret"})
	require.Error(t, err)
}

func TestObjectPathRejectsAbsoluteBucketID(t *testing.T) {
	_, err := objectPath(t.TempDir(), store.StorageObject{BucketID: "/pictures", Name: "avatars/a.png"})
	require.Error(t, err)
}

func TestObjectPathRejectsBucketTraversal(t *testing.T) {
	_, err := objectPath(t.TempDir(), store.StorageObject{BucketID: "pictures/../secret", Name: "avatars/a.png"})
	require.Error(t, err)
}

func TestObjectPathKeepsValidPathsRootedInDirectory(t *testing.T) {
	dir := t.TempDir()
	localPath, err := objectPath(dir, store.StorageObject{BucketID: "pictures", Name: "avatars/a.png"})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "pictures", "avatars", "a.png"), localPath)
}
