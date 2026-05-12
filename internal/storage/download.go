package storage

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/davemorin/supacrawl/internal/store"
)

type Downloader struct {
	BaseURL    string
	AuthToken  string
	HTTPClient *http.Client
	UseCurl    bool
}

type DownloadStats struct {
	Objects int   `json:"objects"`
	Bytes   int64 `json:"bytes"`
	Skipped int   `json:"skipped"`
}

func (d Downloader) DownloadObjects(ctx context.Context, objects []store.StorageObject, dir string, overwrite bool) (DownloadStats, error) {
	client := d.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: 2 * time.Minute,
			Transport: &http.Transport{
				Proxy:               http.ProxyFromEnvironment,
				DialContext:         (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
				TLSHandshakeTimeout: 30 * time.Second,
				TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
			},
		}
	}
	var stats DownloadStats
	for _, object := range objects {
		localPath, err := objectPath(dir, object)
		if err != nil {
			return stats, err
		}
		if !overwrite {
			if info, err := os.Stat(localPath); err == nil && info.Size() > 0 {
				stats.Skipped++
				continue
			}
		}
		if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
			return stats, err
		}
		n, err := d.downloadOne(ctx, client, object, localPath)
		if err != nil {
			return stats, err
		}
		stats.Objects++
		stats.Bytes += n
	}
	return stats, nil
}

func (d Downloader) downloadOne(ctx context.Context, client *http.Client, object store.StorageObject, localPath string) (int64, error) {
	objectURLs, err := d.objectURLs(object)
	if err != nil {
		return 0, err
	}
	var lastErr error
	for _, objectURL := range objectURLs {
		n, err := d.tryDownloadOne(ctx, client, objectURL, object, localPath)
		if err == nil {
			return n, nil
		}
		lastErr = err
	}
	return 0, lastErr
}

func (d Downloader) tryDownloadOne(ctx context.Context, client *http.Client, objectURL string, object store.StorageObject, localPath string) (int64, error) {
	if d.UseCurl {
		return d.tryCurlDownload(ctx, objectURL, localPath, nil)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, objectURL, nil)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(d.AuthToken) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(d.AuthToken))
		req.Header.Set("apikey", strings.TrimSpace(d.AuthToken))
	}
	resp, err := client.Do(req)
	if err != nil {
		return d.tryCurlDownload(ctx, objectURL, localPath, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("download %s/%s: %s", object.BucketID, object.Name, resp.Status)
	}
	tmpPath := localPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return 0, err
	}
	n, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		os.Remove(tmpPath)
		return 0, copyErr
	}
	if closeErr != nil {
		os.Remove(tmpPath)
		return 0, closeErr
	}
	if err := os.Rename(tmpPath, localPath); err != nil {
		os.Remove(tmpPath)
		return 0, err
	}
	return n, nil
}

func (d Downloader) tryCurlDownload(ctx context.Context, objectURL string, localPath string, originalErr error) (int64, error) {
	if _, err := exec.LookPath("curl"); err != nil {
		return 0, originalErr
	}
	tmpPath := localPath + ".tmp"
	args := []string{"-fL", "-sS", "--max-time", "120", "-o", tmpPath, "-K", "-", objectURL}
	cmd := exec.CommandContext(ctx, "curl", args...)
	config := ""
	if strings.TrimSpace(d.AuthToken) != "" {
		token := strings.ReplaceAll(strings.TrimSpace(d.AuthToken), `"`, `\"`)
		config += fmt.Sprintf("header = \"Authorization: Bearer %s\"\n", token)
		config += fmt.Sprintf("header = \"apikey: %s\"\n", token)
	}
	cmd.Stdin = strings.NewReader(config)
	output, err := cmd.CombinedOutput()
	if err != nil {
		os.Remove(tmpPath)
		if originalErr != nil {
			return 0, fmt.Errorf("%w; curl fallback failed: %s", originalErr, strings.TrimSpace(string(output)))
		}
		return 0, fmt.Errorf("curl failed: %s", strings.TrimSpace(string(output)))
	}
	info, err := os.Stat(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return 0, err
	}
	if err := os.Rename(tmpPath, localPath); err != nil {
		os.Remove(tmpPath)
		return 0, err
	}
	return info.Size(), nil
}

func (d Downloader) objectURLs(object store.StorageObject) ([]string, error) {
	standard, err := d.objectURL(object, false)
	if err != nil {
		return nil, err
	}
	authenticated, err := d.objectURL(object, true)
	if err != nil {
		return nil, err
	}
	return []string{standard, authenticated}, nil
}

func (d Downloader) objectURL(object store.StorageObject, authenticated bool) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(d.BaseURL), "/")
	if base == "" {
		return "", fmt.Errorf("supabase URL is empty")
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	parts := []string{"storage", "v1", "object"}
	if authenticated {
		parts = append(parts, "authenticated")
	}
	parts = append(parts, object.BucketID)
	parts = append(parts, strings.Split(object.Name, "/")...)
	parsed.Path = "/" + path.Join(append([]string{strings.TrimPrefix(parsed.Path, "/")}, parts...)...)
	return parsed.String(), nil
}

func objectPath(dir string, object store.StorageObject) (string, error) {
	cleanBucket := filepath.Clean(object.BucketID)
	cleanName := filepath.Clean(object.Name)
	if cleanBucket == "." || cleanBucket == "" || filepath.IsAbs(filepath.FromSlash(object.BucketID)) || strings.HasPrefix(cleanBucket, "..") || hasPathTraversalSegment(object.BucketID) {
		return "", fmt.Errorf("invalid bucket id %q", object.BucketID)
	}
	if cleanName == "." || cleanName == "" || strings.HasPrefix(cleanName, "..") || filepath.IsAbs(filepath.FromSlash(object.Name)) || hasPathTraversalSegment(object.Name) {
		return "", fmt.Errorf("invalid object name %q", object.Name)
	}
	localPath := filepath.Join(dir, cleanBucket, cleanName)
	rel, err := filepath.Rel(dir, localPath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("invalid object path %q/%q", object.BucketID, object.Name)
	}
	return localPath, nil
}

func hasPathTraversalSegment(value string) bool {
	for _, part := range strings.Split(filepath.ToSlash(value), "/") {
		if part == ".." {
			return true
		}
	}
	return false
}
