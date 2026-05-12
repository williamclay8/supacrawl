package management

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadMethodsUseExpectedGETEndpointsAndBearerToken(t *testing.T) {
	tests := []struct {
		name string
		path string
		call func(context.Context, *Client) error
	}{
		{
			name: "list functions",
			path: "/v1/projects/project-ref/functions",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.ListFunctions(ctx, "project-ref")
				return err
			},
		},
		{
			name: "get function body",
			path: "/v1/projects/project-ref/functions/hello%2Fworld/body",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.GetFunctionBody(ctx, "project-ref", "hello/world")
				return err
			},
		},
		{
			name: "list branches",
			path: "/v1/projects/project-ref/branches",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.ListBranches(ctx, "project-ref")
				return err
			},
		},
		{
			name: "list secrets metadata",
			path: "/v1/projects/project-ref/secrets",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.ListSecrets(ctx, "project-ref")
				return err
			},
		},
		{
			name: "get auth config",
			path: "/v1/projects/project-ref/config/auth",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.GetAuthConfig(ctx, "project-ref")
				return err
			},
		},
		{
			name: "get postgrest config",
			path: "/v1/projects/project-ref/postgrest",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.GetPostgRESTConfig(ctx, "project-ref")
				return err
			},
		},
		{
			name: "get storage config",
			path: "/v1/projects/project-ref/config/storage",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.GetStorageConfig(ctx, "project-ref")
				return err
			},
		},
		{
			name: "get realtime config",
			path: "/v1/projects/project-ref/config/realtime",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.GetRealtimeConfig(ctx, "project-ref")
				return err
			},
		},
		{
			name: "list backups",
			path: "/v1/projects/project-ref/database/backups",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.ListBackups(ctx, "project-ref")
				return err
			},
		},
		{
			name: "get security advisor",
			path: "/v1/projects/project-ref/advisors/security",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.GetSecurityAdvisor(ctx, "project-ref")
				return err
			},
		},
		{
			name: "get performance advisor",
			path: "/v1/projects/project-ref/advisors/performance",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.GetPerformanceAdvisor(ctx, "project-ref")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, tt.path, r.URL.EscapedPath())
				require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ok":true}`))
			}))
			defer server.Close()

			client := Client{BaseURL: server.URL, Token: " test-token "}
			require.NoError(t, tt.call(context.Background(), &client))
		})
	}
}

func TestReadMethodsPreserveRawJSON(t *testing.T) {
	raw := json.RawMessage(`{"nested":{"flag":true},"items":[1,2,3]}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, Token: "test-token"}
	got, err := client.GetAuthConfig(context.Background(), "project-ref")
	require.NoError(t, err)
	require.JSONEq(t, string(raw), string(got.Raw))
}

func TestNonSuccessErrorsDoNotExposeTokenOrResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "body contains secret-value and test-token", http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, Token: "test-token"}
	_, err := client.ListFunctions(context.Background(), "project-ref")
	require.Error(t, err)
	require.Contains(t, err.Error(), "429")
	require.Contains(t, err.Error(), "rate limited")
	require.NotContains(t, err.Error(), "test-token")
	require.NotContains(t, err.Error(), "secret-value")
}

func TestCrawlProjectToleratesOptionalNotFound(t *testing.T) {
	optional404 := map[string]bool{
		"/v1/projects/project-ref/postgrest":            true,
		"/v1/projects/project-ref/config/storage":       true,
		"/v1/projects/project-ref/config/realtime":      true,
		"/v1/projects/project-ref/database/backups":     true,
		"/v1/projects/project-ref/advisors/security":    true,
		"/v1/projects/project-ref/advisors/performance": true,
	}
	seen := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path] = true
		if optional404[r.URL.Path] {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/projects/project-ref/functions":
			_, _ = w.Write([]byte(`[{"slug":"hello"}]`))
		case "/v1/projects/project-ref/branches":
			_, _ = w.Write([]byte(`[{"name":"main"}]`))
		case "/v1/projects/project-ref/secrets":
			_, _ = w.Write([]byte(`[{"name":"API_TOKEN","value":"redacted"}]`))
		case "/v1/projects/project-ref/config/auth":
			_, _ = w.Write([]byte(`{"site_url":"https://example.test"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, Token: "test-token"}
	snapshot, err := client.CrawlProject(context.Background(), "project-ref")
	require.NoError(t, err)
	require.Equal(t, "project-ref", snapshot.ProjectRef)
	require.True(t, snapshot.Functions.Available)
	require.True(t, snapshot.Branches.Available)
	require.True(t, snapshot.Secrets.Available)
	require.True(t, snapshot.AuthConfig.Available)
	require.False(t, snapshot.PostgRESTConfig.Available)
	require.False(t, snapshot.StorageConfig.Available)
	require.False(t, snapshot.RealtimeConfig.Available)
	require.False(t, snapshot.Backups.Available)
	require.False(t, snapshot.SecurityAdvisor.Available)
	require.False(t, snapshot.PerformanceAdvisor.Available)
	require.Equal(t, http.StatusNotFound, snapshot.PostgRESTConfig.Status)
	for path := range optional404 {
		require.True(t, seen[path], "expected crawl to request %s", path)
	}
}

func TestCrawlProjectFailsOnAuthErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "do not leak this body", http.StatusForbidden)
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, Token: "test-token"}
	_, err := client.CrawlProject(context.Background(), "project-ref")
	require.Error(t, err)
	require.Contains(t, err.Error(), "403")
	require.NotContains(t, err.Error(), "do not leak")
}

func TestClientRequiresToken(t *testing.T) {
	client := Client{BaseURL: "https://example.test"}
	_, err := client.GetAuthConfig(context.Background(), "project-ref")
	require.Error(t, err)
	require.Contains(t, err.Error(), "token")
}

func TestDefaultBaseURL(t *testing.T) {
	client := NewClient("test-token")
	require.Equal(t, DefaultBaseURL, client.BaseURL)
	require.Equal(t, "https://api.supabase.com", client.BaseURL)
}

func TestStatusErrorSupportsErrorsAs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, Token: "test-token"}
	_, err := client.GetStorageConfig(context.Background(), "project-ref")
	var statusErr *StatusError
	require.True(t, errors.As(err, &statusErr), fmt.Sprintf("expected StatusError, got %T", err))
	require.Equal(t, http.StatusNotFound, statusErr.StatusCode)
	require.Equal(t, "GET", statusErr.Method)
	require.Equal(t, "/v1/projects/project-ref/config/storage", statusErr.Path)
	require.False(t, strings.Contains(statusErr.Error(), "test-token"))
}
