package management

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.supabase.com"

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		Token:   token,
	}
}

type StatusError struct {
	Method     string
	Path       string
	StatusCode int
	Status     string
}

func (e *StatusError) Error() string {
	message := "request failed"
	if e.StatusCode == http.StatusTooManyRequests {
		message = "rate limited"
	}
	return fmt.Sprintf("%s %s: %s (%d)", e.Method, e.Path, message, e.StatusCode)
}

type Functions struct {
	Raw json.RawMessage `json:"raw"`
}

type FunctionBody struct {
	Raw json.RawMessage `json:"raw"`
}

type Branches struct {
	Raw json.RawMessage `json:"raw"`
}

type SecretsMetadata struct {
	Items []SecretMetadata `json:"items"`
	Raw   json.RawMessage  `json:"raw"`
}

type SecretMetadata struct {
	Name      string          `json:"name,omitempty"`
	UpdatedAt string          `json:"updated_at,omitempty"`
	Raw       json.RawMessage `json:"raw,omitempty"`
}

type AuthConfig struct {
	Raw json.RawMessage `json:"raw"`
}

type PostgRESTConfig struct {
	Raw json.RawMessage `json:"raw"`
}

type StorageConfig struct {
	Raw json.RawMessage `json:"raw"`
}

type RealtimeConfig struct {
	Raw json.RawMessage `json:"raw"`
}

type Backups struct {
	Raw json.RawMessage `json:"raw"`
}

type Advisor struct {
	Raw json.RawMessage `json:"raw"`
}

type Snapshot struct {
	ProjectRef         string          `json:"project_ref"`
	Functions          SnapshotSection `json:"functions"`
	Branches           SnapshotSection `json:"branches"`
	Secrets            SnapshotSection `json:"secrets"`
	AuthConfig         SnapshotSection `json:"auth_config"`
	PostgRESTConfig    SnapshotSection `json:"postgrest_config"`
	StorageConfig      SnapshotSection `json:"storage_config"`
	RealtimeConfig     SnapshotSection `json:"realtime_config"`
	Backups            SnapshotSection `json:"backups"`
	SecurityAdvisor    SnapshotSection `json:"security_advisor"`
	PerformanceAdvisor SnapshotSection `json:"performance_advisor"`
}

type SnapshotSection struct {
	Available bool            `json:"available"`
	Status    int             `json:"status,omitempty"`
	Raw       json.RawMessage `json:"raw,omitempty"`
}

func (c *Client) ListFunctions(ctx context.Context, ref string) (Functions, error) {
	raw, err := c.get(ctx, projectPath(ref, "functions"))
	return Functions{Raw: raw}, err
}

func (c *Client) GetFunctionBody(ctx context.Context, ref, slug string) (FunctionBody, error) {
	raw, err := c.get(ctx, projectPath(ref, "functions", slug, "body"))
	return FunctionBody{Raw: raw}, err
}

func (c *Client) ListBranches(ctx context.Context, ref string) (Branches, error) {
	raw, err := c.get(ctx, projectPath(ref, "branches"))
	return Branches{Raw: raw}, err
}

func (c *Client) ListSecrets(ctx context.Context, ref string) (SecretsMetadata, error) {
	raw, err := c.get(ctx, projectPath(ref, "secrets"))
	if err != nil {
		return SecretsMetadata{}, err
	}
	return scrubSecrets(raw)
}

func (c *Client) GetAuthConfig(ctx context.Context, ref string) (AuthConfig, error) {
	raw, err := c.get(ctx, projectPath(ref, "config", "auth"))
	return AuthConfig{Raw: raw}, err
}

func (c *Client) GetPostgRESTConfig(ctx context.Context, ref string) (PostgRESTConfig, error) {
	raw, err := c.get(ctx, projectPath(ref, "postgrest"))
	return PostgRESTConfig{Raw: raw}, err
}

func (c *Client) GetStorageConfig(ctx context.Context, ref string) (StorageConfig, error) {
	raw, err := c.get(ctx, projectPath(ref, "config", "storage"))
	return StorageConfig{Raw: raw}, err
}

func (c *Client) GetRealtimeConfig(ctx context.Context, ref string) (RealtimeConfig, error) {
	raw, err := c.get(ctx, projectPath(ref, "config", "realtime"))
	return RealtimeConfig{Raw: raw}, err
}

func (c *Client) ListBackups(ctx context.Context, ref string) (Backups, error) {
	raw, err := c.get(ctx, projectPath(ref, "database", "backups"))
	return Backups{Raw: raw}, err
}

func (c *Client) GetSecurityAdvisor(ctx context.Context, ref string) (Advisor, error) {
	raw, err := c.get(ctx, projectPath(ref, "advisors", "security"))
	return Advisor{Raw: raw}, err
}

func (c *Client) GetPerformanceAdvisor(ctx context.Context, ref string) (Advisor, error) {
	raw, err := c.get(ctx, projectPath(ref, "advisors", "performance"))
	return Advisor{Raw: raw}, err
}

func (c *Client) CrawlProject(ctx context.Context, ref string) (Snapshot, error) {
	snapshot := Snapshot{ProjectRef: ref}
	steps := []struct {
		target *SnapshotSection
		call   func(context.Context, string) (json.RawMessage, error)
	}{
		{&snapshot.Functions, func(ctx context.Context, ref string) (json.RawMessage, error) {
			v, err := c.ListFunctions(ctx, ref)
			return v.Raw, err
		}},
		{&snapshot.Branches, func(ctx context.Context, ref string) (json.RawMessage, error) {
			v, err := c.ListBranches(ctx, ref)
			return v.Raw, err
		}},
		{&snapshot.Secrets, func(ctx context.Context, ref string) (json.RawMessage, error) {
			v, err := c.ListSecrets(ctx, ref)
			return v.Raw, err
		}},
		{&snapshot.AuthConfig, func(ctx context.Context, ref string) (json.RawMessage, error) {
			v, err := c.GetAuthConfig(ctx, ref)
			return v.Raw, err
		}},
		{&snapshot.PostgRESTConfig, func(ctx context.Context, ref string) (json.RawMessage, error) {
			v, err := c.GetPostgRESTConfig(ctx, ref)
			return v.Raw, err
		}},
		{&snapshot.StorageConfig, func(ctx context.Context, ref string) (json.RawMessage, error) {
			v, err := c.GetStorageConfig(ctx, ref)
			return v.Raw, err
		}},
		{&snapshot.RealtimeConfig, func(ctx context.Context, ref string) (json.RawMessage, error) {
			v, err := c.GetRealtimeConfig(ctx, ref)
			return v.Raw, err
		}},
		{&snapshot.Backups, func(ctx context.Context, ref string) (json.RawMessage, error) {
			v, err := c.ListBackups(ctx, ref)
			return v.Raw, err
		}},
		{&snapshot.SecurityAdvisor, func(ctx context.Context, ref string) (json.RawMessage, error) {
			v, err := c.GetSecurityAdvisor(ctx, ref)
			return v.Raw, err
		}},
		{&snapshot.PerformanceAdvisor, func(ctx context.Context, ref string) (json.RawMessage, error) {
			v, err := c.GetPerformanceAdvisor(ctx, ref)
			return v.Raw, err
		}},
	}
	for _, step := range steps {
		raw, err := step.call(ctx, ref)
		if err != nil {
			statusErr, ok := err.(*StatusError)
			if ok && statusErr.StatusCode == http.StatusNotFound {
				*step.target = SnapshotSection{Available: false, Status: statusErr.StatusCode}
				continue
			}
			return snapshot, err
		}
		*step.target = SnapshotSection{Available: true, Status: http.StatusOK, Raw: raw}
	}
	return snapshot, nil
}

func (c *Client) get(ctx context.Context, requestPath string) (json.RawMessage, error) {
	token := strings.TrimSpace(c.Token)
	if token == "" {
		return nil, fmt.Errorf("management API token is required")
	}
	endpoint, err := c.url(requestPath)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, &StatusError{
			Method:     req.Method,
			Path:       req.URL.EscapedPath(),
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
		}
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

func (c *Client) url(requestPath string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		base = DefaultBaseURL
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(parsed.String(), "/") + requestPath, nil
}

func projectPath(ref string, parts ...string) string {
	segments := []string{"v1", "projects", ref}
	segments = append(segments, parts...)
	escaped := make([]string, 0, len(segments))
	for _, segment := range segments {
		escaped = append(escaped, url.PathEscape(segment))
	}
	return "/" + strings.Join(escaped, "/")
}

func scrubSecrets(raw json.RawMessage) (SecretsMetadata, error) {
	var secrets []map[string]any
	if err := json.Unmarshal(raw, &secrets); err != nil {
		return SecretsMetadata{Raw: raw}, nil
	}
	items := make([]SecretMetadata, 0, len(secrets))
	scrubbed := make([]map[string]any, 0, len(secrets))
	for _, secret := range secrets {
		delete(secret, "value")
		itemRaw, err := json.Marshal(secret)
		if err != nil {
			return SecretsMetadata{}, err
		}
		items = append(items, SecretMetadata{
			Name:      stringField(secret, "name"),
			UpdatedAt: stringField(secret, "updated_at"),
			Raw:       itemRaw,
		})
		scrubbed = append(scrubbed, secret)
	}
	scrubbedRaw, err := json.Marshal(scrubbed)
	if err != nil {
		return SecretsMetadata{}, err
	}
	return SecretsMetadata{Items: items, Raw: scrubbedRaw}, nil
}

func stringField(values map[string]any, key string) string {
	value, ok := values[key].(string)
	if !ok {
		return ""
	}
	return value
}
