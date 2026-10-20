package rancher

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListClusters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/clusters" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("unexpected auth header: %s", got)
		}
		_ = json.NewEncoder(w).Encode(clusterListResponse{
			Data: []Cluster{{ID: "c-abc", Name: "prod", State: "active"}},
		})
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL, "test-token", false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	clusters, err := client.ListClusters(context.Background())
	if err != nil {
		t.Fatalf("ListClusters: %v", err)
	}
	if len(clusters) != 1 || clusters[0].ID != "c-abc" {
		t.Fatalf("unexpected clusters: %+v", clusters)
	}
}

func TestListClusters401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL, "bad-token", false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.ListClusters(context.Background())
	if !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expected ErrTokenInvalid, got %v", err)
	}
}

func TestValidateToken401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL, "bad-token", false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	err = client.ValidateToken(context.Background())
	if !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expected ErrTokenInvalid, got %v", err)
	}
}

func TestGenerateKubeconfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v3/clusters/c-abc" || r.URL.Query().Get("action") != "generateKubeconfig" {
			t.Fatalf("unexpected URL: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(kubeconfigResponse{Config: "apiVersion: v1\nkind: Config\n"})
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL, "test-token", false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	cfg, err := client.GenerateKubeconfig(context.Background(), "c-abc")
	if err != nil {
		t.Fatalf("GenerateKubeconfig: %v", err)
	}
	if cfg == "" {
		t.Fatal("expected non-empty kubeconfig")
	}
}

func TestGenerateKubeconfig401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL, "bad-token", false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.GenerateKubeconfig(context.Background(), "c-abc")
	if !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expected ErrTokenInvalid, got %v", err)
	}
}

func TestGenerateKubeconfigNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"cluster not found"}`))
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL, "test-token", false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.GenerateKubeconfig(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for missing cluster")
	}
	if errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expected non-auth error, got %v", err)
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected status in error, got %v", err)
	}
}

func TestNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	client, err := NewClient(url, "test-token", false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	err = client.ValidateToken(context.Background())
	if err == nil {
		t.Fatal("expected network error")
	}
	if !strings.Contains(err.Error(), "rancher API request failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGenerateKubeconfigEmptyID(t *testing.T) {
	client, err := NewClient("https://example.com", "token", false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.GenerateKubeconfig(context.Background(), "  ")
	if err == nil || !strings.Contains(err.Error(), "cluster ID must not be empty") {
		t.Fatalf("expected empty ID error, got %v", err)
	}
}

func TestInsecureSkipVerify(t *testing.T) {
	client, err := NewClient("https://example.com", "token", true)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if !client.insecureSkipVerify {
		t.Fatal("expected insecureSkipVerify to be enabled")
	}
}

func TestLoginAuthProviderRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3-public/activeDirectoryProviders/activeDirectory" || r.URL.Query().Get("action") != "login" {
			t.Fatalf("unexpected URL: %s", r.URL.String())
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL, "", false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.LoginAuthProvider(context.Background(), "activeDirectory", "activeDirectory", "alice", "wrong")
	if !errors.Is(err, ErrLoginFailed) {
		t.Fatalf("expected ErrLoginFailed, got %v", err)
	}
	if errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("login failure must not look like an expired token: %v", err)
	}
}

func TestListAuthProviders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3-public/authProviders" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("public endpoint must be called without a token")
		}
		_, _ = w.Write([]byte(`{"data":[
			{"id":"local","type":"localProvider"},
			{"id":"openldap","type":"openLdapProvider"},
			{"id":"keycloakoidc","type":"keyCloakOIDCProvider"}]}`))
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL, "", false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	providers, err := client.ListAuthProviders(context.Background())
	if err != nil {
		t.Fatalf("ListAuthProviders: %v", err)
	}
	if len(providers) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(providers))
	}

	pw := PasswordLoginProviders(providers)
	if len(pw) != 2 || pw[0].ID != "local" || pw[1].ID != "openldap" {
		t.Fatalf("unexpected password providers: %+v", pw)
	}
	if got := pw[1].LoginType(); got != "openLdap" {
		t.Fatalf("expected login type openLdap, got %q", got)
	}
}
