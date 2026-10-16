package rancher

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second

// Cluster represents a Rancher-managed cluster.
type Cluster struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

type clusterListResponse struct {
	Data []Cluster `json:"data"`
}

type kubeconfigResponse struct {
	Config string `json:"config"`
}

type loginResponse struct {
	Token string `json:"token"`
}

type tokenResponse struct {
	Token string `json:"token"`
}

// Client talks to the Rancher API.
type Client struct {
	baseURL            string
	token              string
	insecureSkipVerify bool
	httpClient         *http.Client
}

// NewClient creates a Rancher API client.
func NewClient(baseURL, token string, insecureSkipVerify bool) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("rancher URL must not be empty")
	}
	if _, err := url.Parse(baseURL); err != nil {
		return nil, fmt.Errorf("invalid rancher URL: %w", err)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: insecureSkipVerify, //nolint:gosec // user-controlled per instance
	}

	return &Client{
		baseURL:            baseURL,
		token:              token,
		insecureSkipVerify: insecureSkipVerify,
		httpClient: &http.Client{
			Timeout:   defaultTimeout,
			Transport: transport,
		},
	}, nil
}

// LoginAuthProvider authenticates against a Rancher auth provider and returns a session token.
func (c *Client) LoginAuthProvider(ctx context.Context, providerType, providerID, username, password string) (string, error) {
	providerType = strings.TrimSpace(providerType)
	providerID = strings.TrimSpace(providerID)
	username = strings.TrimSpace(username)
	if providerType == "" {
		return "", fmt.Errorf("auth provider type must not be empty")
	}
	if providerID == "" {
		return "", fmt.Errorf("auth provider ID must not be empty")
	}
	if username == "" {
		return "", fmt.Errorf("auth username must not be empty")
	}
	if password == "" {
		return "", fmt.Errorf("auth password must not be empty")
	}

	body := map[string]string{
		"username":     username,
		"password":     password,
		"responseType": "json",
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal auth login request: %w", err)
	}

	path := fmt.Sprintf("/v3-public/%sProviders/%s?action=login", providerType, url.PathEscape(providerID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return "", ErrLoginFailed
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", c.readError(resp)
	}

	var out loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode auth login response: %w", err)
	}
	if strings.TrimSpace(out.Token) == "" {
		return "", fmt.Errorf("rancher returned empty auth login token")
	}
	return out.Token, nil
}

// CreateAPIToken creates a long-lived Rancher API token from an authenticated session token.
func (c *Client) CreateAPIToken(ctx context.Context, description string) (string, error) {
	description = strings.TrimSpace(description)
	if description == "" {
		description = "kubectl-sheep"
	}

	body := map[string]any{
		"type":        "token",
		"description": description,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal token create request: %w", err)
	}

	req, err := c.newRequest(ctx, http.MethodPost, "/v3/tokens", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}

	resp, err := c.do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return "", ErrTokenInvalid
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", c.readError(resp)
	}

	var out tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode token create response: %w", err)
	}
	if strings.TrimSpace(out.Token) == "" {
		return "", fmt.Errorf("rancher returned empty API token")
	}
	return out.Token, nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	endpoint := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rancher API request failed: %w", err)
	}
	return resp, nil
}

func (c *Client) readError(resp *http.Response) error {
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrTokenInvalid
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		return fmt.Errorf("rancher API returned status %d", resp.StatusCode)
	}
	return fmt.Errorf("rancher API returned status %d: %s", resp.StatusCode, msg)
}

// ValidateToken checks whether the token is accepted by the Rancher API.
func (c *Client) ValidateToken(ctx context.Context) error {
	req, err := c.newRequest(ctx, http.MethodGet, "/v3/clusters?limit=1", nil)
	if err != nil {
		return err
	}

	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return ErrTokenInvalid
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.readError(resp)
	}
	return nil
}

// ListClusters returns all clusters from GET /v3/clusters.
func (c *Client) ListClusters(ctx context.Context) ([]Cluster, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/v3/clusters", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrTokenInvalid
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, c.readError(resp)
	}

	var list clusterListResponse
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("decode cluster list: %w", err)
	}
	return list.Data, nil
}

// GenerateKubeconfig calls POST /v3/clusters/<id>?action=generateKubeconfig.
func (c *Client) GenerateKubeconfig(ctx context.Context, clusterID string) (string, error) {
	clusterID = strings.TrimSpace(clusterID)
	if clusterID == "" {
		return "", fmt.Errorf("cluster ID must not be empty")
	}

	path := fmt.Sprintf("/v3/clusters/%s?action=generateKubeconfig", clusterID)
	req, err := c.newRequest(ctx, http.MethodPost, path, bytes.NewReader([]byte("{}")))
	if err != nil {
		return "", err
	}

	resp, err := c.do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return "", ErrTokenInvalid
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", c.readError(resp)
	}

	var out kubeconfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode kubeconfig response: %w", err)
	}
	if strings.TrimSpace(out.Config) == "" {
		return "", fmt.Errorf("rancher returned empty kubeconfig for cluster %q", clusterID)
	}
	return out.Config, nil
}
