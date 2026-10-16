package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/ahoz/kubectl-sheep/internal/config"
	"github.com/ahoz/kubectl-sheep/internal/credentials"
	"github.com/ahoz/kubectl-sheep/internal/kubeconfig"
)

const (
	fakeOldToken     = "old-token"
	fakeSessionToken = "session-token"
	fakeNewToken     = "new-token"
	fakePassword     = "s3cret"
)

// fakeRancher simulates the handful of Rancher endpoints connect relies on.
type fakeRancher struct {
	mu          sync.Mutex
	validTokens map[string]bool
	generated   int
	logins      int
	kubeconfig  string
}

func newFakeRancher(t *testing.T) (*fakeRancher, *httptest.Server) {
	t.Helper()
	f := &fakeRancher{
		validTokens: map[string]bool{fakeNewToken: true},
		kubeconfig:  fakeKubeconfig("v1"),
	}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, srv
}

func fakeKubeconfig(token string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: c-1
  cluster:
    server: https://cluster.example.com
contexts:
- name: c-1
  context:
    cluster: c-1
    user: c-1
current-context: c-1
users:
- name: c-1
  user:
    token: %s
`, token)
}

func (f *fakeRancher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if r.URL.Path == "/v3-public/activeDirectoryProviders/activeDirectory" && r.URL.Query().Get("action") == "login" {
		f.logins++
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["username"] != "alice" || body["password"] != fakePassword {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"token": fakeSessionToken})
		return
	}

	bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")

	if r.URL.Path == "/v3/tokens" && r.Method == http.MethodPost {
		if bearer != fakeSessionToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"token": fakeNewToken})
		return
	}

	if !f.validTokens[bearer] {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	switch {
	case r.URL.Path == "/v3/clusters":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "c-1", "name": "prod-cluster", "state": "active"}},
		})
	case r.URL.Path == "/v3/clusters/c-1" && r.URL.Query().Get("action") == "generateKubeconfig":
		f.generated++
		_ = json.NewEncoder(w).Encode(map[string]string{"config": f.kubeconfig})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// setupConnectHome points HOME at a temp dir with one plaintext instance,
// a stored (stale) token and one stored kubeconfig.
func setupConnectHome(t *testing.T, serverURL string, auth *config.AuthLogin) *config.Instance {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KUBECONFIG", "")

	inst := config.Instance{
		Name:    "prod",
		URL:     serverURL,
		Storage: config.StoragePlaintext,
		Auth:    auth,
	}
	cfg := &config.Config{Instances: []config.Instance{inst}}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	store, err := credentials.NewStore(config.StoragePlaintext)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Set("prod", fakeOldToken); err != nil {
		t.Fatalf("Set token: %v", err)
	}

	if err := kubeconfig.Save("prod", "c-1", "prod-cluster", fakeKubeconfig("v0")); err != nil {
		t.Fatalf("Save kubeconfig: %v", err)
	}
	if err := mergeKubeconfig("prod", "prod-cluster", fakeKubeconfig("v0")); err != nil {
		t.Fatalf("mergeKubeconfig: %v", err)
	}
	return &inst
}

func adLogin() *config.AuthLogin {
	return &config.AuthLogin{ProviderType: "activeDirectory", ProviderID: "activeDirectory", Username: "alice"}
}

func storedToken(t *testing.T) string {
	t.Helper()
	store, err := credentials.NewStore(config.StoragePlaintext)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	token, err := store.Get("prod")
	if err != nil {
		t.Fatalf("Get token: %v", err)
	}
	return token
}

func TestConnectRenewsExpiredTokenAndRefreshes(t *testing.T) {
	fake, srv := newFakeRancher(t)
	inst := setupConnectHome(t, srv.URL, adLogin())

	var out bytes.Buffer
	prompts := 0
	s := newConnectSession(&out, false, false)
	s.readPassword = func(label, hint string) (string, error) {
		prompts++
		if !strings.Contains(label, "alice") {
			t.Errorf("prompt label should name the user, got %q", label)
		}
		if !strings.Contains(hint, "expired") {
			t.Errorf("prompt hint should explain why, got %q", hint)
		}
		return fakePassword, nil
	}

	if err := s.connectInstance(context.Background(), inst); err != nil {
		t.Fatalf("connectInstance: %v\n%s", err, out.String())
	}

	if prompts != 1 {
		t.Fatalf("expected 1 password prompt, got %d", prompts)
	}
	if got := storedToken(t); got != fakeNewToken {
		t.Fatalf("expected renewed token stored, got %q", got)
	}
	if fake.generated != 1 {
		t.Fatalf("expected 1 kubeconfig refresh, got %d", fake.generated)
	}

	path, _ := kubeconfig.KubeconfigPath("prod", "c-1")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "token: v1") {
		t.Fatalf("stored kubeconfig not refreshed:\n%s", data)
	}

	text := out.String()
	if !strings.Contains(text, "token renewed via activeDirectory (alice)") {
		t.Fatalf("expected renewal message, got:\n%s", text)
	}
	if !strings.Contains(text, "1 kubeconfig refreshed, 1 updated") {
		t.Fatalf("expected refresh summary, got:\n%s", text)
	}
}

func TestConnectValidTokenNoPrompt(t *testing.T) {
	fake, srv := newFakeRancher(t)
	fake.validTokens[fakeOldToken] = true
	inst := setupConnectHome(t, srv.URL, adLogin())

	var out bytes.Buffer
	s := newConnectSession(&out, false, false)
	s.readPassword = func(label, hint string) (string, error) {
		t.Fatal("password prompt must not be shown for a valid token")
		return "", nil
	}

	if err := s.connectInstance(context.Background(), inst); err != nil {
		t.Fatalf("connectInstance: %v", err)
	}
	if got := storedToken(t); got != fakeOldToken {
		t.Fatalf("token must be kept, got %q", got)
	}
	if fake.logins != 0 {
		t.Fatalf("expected no login, got %d", fake.logins)
	}
	if !strings.Contains(out.String(), "✓ prod: connected") {
		t.Fatalf("expected connected line, got:\n%s", out.String())
	}
}

func TestConnectQuietPrintsNothingWhenHealthy(t *testing.T) {
	fake, srv := newFakeRancher(t)
	fake.validTokens[fakeOldToken] = true
	inst := setupConnectHome(t, srv.URL, adLogin())

	var out bytes.Buffer
	s := newConnectSession(&out, true, false)
	if err := s.connectInstance(context.Background(), inst); err != nil {
		t.Fatalf("connectInstance: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("quiet mode should print nothing, got:\n%s", out.String())
	}
}

func TestConnectPasswordSharedAcrossInstances(t *testing.T) {
	_, srv := newFakeRancher(t)
	inst := setupConnectHome(t, srv.URL, adLogin())

	// Second instance with the same login against the same AD.
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	second := config.Instance{Name: "dev", URL: srv.URL, Storage: config.StoragePlaintext, Auth: adLogin()}
	if err := cfg.AddInstance(second); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	prompts := 0
	s := newConnectSession(&out, false, true)
	s.readPassword = func(label, hint string) (string, error) {
		prompts++
		return fakePassword, nil
	}

	if err := s.connectInstance(context.Background(), inst); err != nil {
		t.Fatalf("connect prod: %v", err)
	}
	if err := s.connectInstance(context.Background(), &second); err != nil {
		t.Fatalf("connect dev: %v", err)
	}
	if prompts != 1 {
		t.Fatalf("expected a single password prompt for both instances, got %d", prompts)
	}
}

func TestConnectWrongPasswordRetries(t *testing.T) {
	fake, srv := newFakeRancher(t)
	inst := setupConnectHome(t, srv.URL, adLogin())

	var out bytes.Buffer
	answers := []string{"wrong", fakePassword}
	s := newConnectSession(&out, false, true)
	s.readPassword = func(label, hint string) (string, error) {
		next := answers[0]
		answers = answers[1:]
		return next, nil
	}

	if err := s.connectInstance(context.Background(), inst); err != nil {
		t.Fatalf("connectInstance: %v\n%s", err, out.String())
	}
	if len(answers) != 0 {
		t.Fatalf("expected both answers to be consumed")
	}
	if fake.logins != 2 {
		t.Fatalf("expected 2 login attempts, got %d", fake.logins)
	}
	if !strings.Contains(out.String(), "prod: login as alice failed") {
		t.Fatalf("expected failed login notice, got:\n%s", out.String())
	}
	if got := storedToken(t); got != fakeNewToken {
		t.Fatalf("expected renewed token, got %q", got)
	}
}

func TestConnectGivesUpAfterMaxAttempts(t *testing.T) {
	fake, srv := newFakeRancher(t)
	inst := setupConnectHome(t, srv.URL, adLogin())

	var out bytes.Buffer
	s := newConnectSession(&out, false, true)
	s.readPassword = func(label, hint string) (string, error) {
		return "wrong", nil
	}

	err := s.connectInstance(context.Background(), inst)
	if err == nil || !strings.Contains(err.Error(), "after 3 attempts") {
		t.Fatalf("expected max attempts error, got %v", err)
	}
	if fake.logins != maxPasswordAttempts {
		t.Fatalf("expected %d login attempts, got %d", maxPasswordAttempts, fake.logins)
	}
	if got := storedToken(t); got != fakeOldToken {
		t.Fatalf("old token must be kept on failure, got %q", got)
	}
}

func TestConnectWithoutAuthLoginPointsToUpdateToken(t *testing.T) {
	_, srv := newFakeRancher(t)
	inst := setupConnectHome(t, srv.URL, nil)

	var out bytes.Buffer
	s := newConnectSession(&out, false, false)
	s.readPassword = func(label, hint string) (string, error) {
		t.Fatal("no prompt expected without stored auth login")
		return "", nil
	}

	err := s.connectInstance(context.Background(), inst)
	if err == nil || !strings.Contains(err.Error(), "rancher-instance update-token prod") {
		t.Fatalf("expected update-token hint, got %v", err)
	}
}

func TestConnectNonInteractiveDoesNotPrompt(t *testing.T) {
	fake, srv := newFakeRancher(t)
	inst := setupConnectHome(t, srv.URL, adLogin())

	var out bytes.Buffer
	s := newConnectSession(&out, false, false) // readPassword nil → non-interactive

	err := s.connectInstance(context.Background(), inst)
	if err == nil || !strings.Contains(err.Error(), "on a terminal") {
		t.Fatalf("expected non-interactive hint, got %v", err)
	}
	if fake.logins != 0 {
		t.Fatalf("expected no login attempts, got %d", fake.logins)
	}
}

func TestConnectSkipsExecContexts(t *testing.T) {
	fake, srv := newFakeRancher(t)
	fake.validTokens[fakeOldToken] = true
	inst := setupConnectHome(t, srv.URL, adLogin())

	// Replace the merged context with an exec-based one, as install-exec does.
	execContent, err := buildExecKubeconfig(fakeKubeconfig("v0"), execKubeconfigOptions{
		contextName: mergeContextName("prod", "prod-cluster"),
		command:     "kubectl-sheep",
		args:        []string{"auth", "exec", "prod", "c-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mergeKubeconfig("prod", "prod-cluster", execContent); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	s := newConnectSession(&out, false, false)
	if err := s.connectInstance(context.Background(), inst); err != nil {
		t.Fatalf("connectInstance: %v", err)
	}
	if fake.generated != 0 {
		t.Fatalf("exec contexts must not be refreshed, got %d generateKubeconfig calls", fake.generated)
	}
	if !strings.Contains(out.String(), "1 exec context on demand") {
		t.Fatalf("expected exec summary, got:\n%s", out.String())
	}

	configPath, _ := kubeconfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "kubectl-sheep") {
		t.Fatalf("exec context was replaced:\n%s", data)
	}
}

func TestSelectConnectInstances(t *testing.T) {
	cfg := &config.Config{Instances: []config.Instance{{Name: "a"}, {Name: "b"}}}

	all, err := selectConnectInstances(cfg, nil)
	if err != nil || len(all) != 2 {
		t.Fatalf("expected all instances, got %v, %v", all, err)
	}

	some, err := selectConnectInstances(cfg, []string{"b", "b"})
	if err != nil || len(some) != 1 || some[0].Name != "b" {
		t.Fatalf("expected deduplicated selection, got %v, %v", some, err)
	}

	if _, err := selectConnectInstances(cfg, []string{"missing"}); err == nil {
		t.Fatal("expected error for unknown instance")
	}
}

func TestConnectNetworkErrorIsNotTreatedAsExpired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	inst := setupConnectHome(t, url, adLogin())

	var out bytes.Buffer
	s := newConnectSession(&out, false, false)
	s.readPassword = func(label, hint string) (string, error) {
		t.Fatal("network errors must not trigger a password prompt")
		return "", nil
	}

	err := s.connectInstance(context.Background(), inst)
	if err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("expected network error, got %v", err)
	}
	if got := storedToken(t); got != fakeOldToken {
		t.Fatalf("token must be kept, got %q", got)
	}
}

// A second Rancher bound to another directory: same username, other password.
func TestConnectSecondDirectoryAsksAgainAndRemembersBoth(t *testing.T) {
	_, srv := newFakeRancher(t)
	inst := setupConnectHome(t, srv.URL, adLogin())

	otherPassword := "other-ad"
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v3-public/"):
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["password"] != otherPassword {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "other-session"})
		case r.URL.Path == "/v3/tokens":
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "other-token"})
		case r.URL.Path == "/v3/clusters":
			if r.Header.Get("Authorization") != "Bearer other-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(other.Close)

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	legacy := config.Instance{Name: "legacy", URL: other.URL, Storage: config.StoragePlaintext, Auth: adLogin()}
	third := config.Instance{Name: "dev", URL: srv.URL, Storage: config.StoragePlaintext, Auth: adLogin()}
	for _, i := range []config.Instance{legacy, third} {
		if err := cfg.AddInstance(i); err != nil {
			t.Fatal(err)
		}
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var prompts []string
	answers := []string{fakePassword, otherPassword}
	s := newConnectSession(&out, false, true)
	s.readPassword = func(label, hint string) (string, error) {
		prompts = append(prompts, label)
		next := answers[0]
		answers = answers[1:]
		return next, nil
	}

	// prod → first prompt; legacy → known password rejected, second prompt;
	// dev → both known passwords tried, the first one works, no prompt.
	for _, i := range []*config.Instance{inst, &legacy, &third} {
		if err := s.connectInstance(context.Background(), i); err != nil {
			t.Fatalf("connect %s: %v\n%s", i.Name, err, out.String())
		}
	}

	if len(prompts) != 2 {
		t.Fatalf("expected exactly 2 prompts, got %d: %v", len(prompts), prompts)
	}
	if !strings.Contains(prompts[1], "legacy") {
		t.Fatalf("second prompt should name the instance, got %q", prompts[1])
	}
	if !strings.Contains(out.String(), "legacy: the password used so far is not accepted") {
		t.Fatalf("expected notice about the other directory, got:\n%s", out.String())
	}
	if len(s.passwords[adLogin().Key()]) != 2 {
		t.Fatalf("expected both passwords remembered, got %d", len(s.passwords[adLogin().Key()]))
	}
}
