package cmd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahoz/kubectl-sheep/internal/config"
	"github.com/ahoz/kubectl-sheep/internal/rancher"
)

func providerServer(t *testing.T, providersJSON string, login http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v3-public/authProviders":
			_, _ = w.Write([]byte(providersJSON))
		case strings.HasPrefix(r.URL.Path, "/v3-public/") && login != nil:
			login(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func autoLogin() *config.AuthLogin {
	return &config.AuthLogin{ProviderType: "auto", ProviderID: "auto", Username: "alice"}
}

func TestResolveAutoAuthProviderPicksDirectory(t *testing.T) {
	srv := providerServer(t, `{"data":[{"id":"local","type":"localProvider"},{"id":"openldap","type":"openLdapProvider"},{"id":"keycloakoidc","type":"keyCloakOIDCProvider"}]}`, nil)

	got, err := resolveAutoAuthProvider(context.Background(), srv.URL, false, autoLogin())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.ProviderType != "openLdap" || got.ProviderID != "openldap" || got.Username != "alice" {
		t.Fatalf("unexpected resolution: %+v", got)
	}
}

func TestResolveAutoAuthProviderFallsBackToLocal(t *testing.T) {
	srv := providerServer(t, `{"data":[{"id":"local","type":"localProvider"}]}`, nil)

	got, err := resolveAutoAuthProvider(context.Background(), srv.URL, false, autoLogin())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.ProviderType != "local" || got.ProviderID != "local" {
		t.Fatalf("unexpected resolution: %+v", got)
	}
}

func TestResolveAutoAuthProviderAmbiguous(t *testing.T) {
	srv := providerServer(t, `{"data":[{"id":"openldap","type":"openLdapProvider"},{"id":"activeDirectory","type":"activeDirectoryProvider"}]}`, nil)

	_, err := resolveAutoAuthProvider(context.Background(), srv.URL, false, autoLogin())
	if err == nil || !strings.Contains(err.Error(), "--auth-provider-id activeDirectory") || !strings.Contains(err.Error(), "--auth-provider-id openldap") {
		t.Fatalf("expected error listing both providers, got %v", err)
	}
}

func TestResolveAutoAuthProviderNoneUsable(t *testing.T) {
	srv := providerServer(t, `{"data":[{"id":"keycloakoidc","type":"keyCloakOIDCProvider"}]}`, nil)

	_, err := resolveAutoAuthProvider(context.Background(), srv.URL, false, autoLogin())
	if err == nil || !strings.Contains(err.Error(), "no auth provider with password login") {
		t.Fatalf("expected no-provider error, got %v", err)
	}
}

func TestResolveAutoAuthProviderKeepsExplicit(t *testing.T) {
	explicit := &config.AuthLogin{ProviderType: "activeDirectory", ProviderID: "activeDirectory", Username: "alice"}
	got, err := resolveAutoAuthProvider(context.Background(), "https://unreachable.invalid", false, explicit)
	if err != nil {
		t.Fatalf("explicit settings must not hit the network: %v", err)
	}
	if got != explicit {
		t.Fatalf("explicit settings must be returned unchanged")
	}
}

func TestCreateTokenUnknownProviderListsAlternatives(t *testing.T) {
	srv := providerServer(t, `{"data":[{"id":"openldap","type":"openLdapProvider"}]}`, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"authconfigs.management.cattle.io \"activeDirectory\" not found"}`))
	})

	explicit := &config.AuthLogin{ProviderType: "activeDirectory", ProviderID: "activeDirectory", Username: "alice"}
	_, err := createTokenViaAuthLogin(context.Background(), srv.URL, false, explicit, "pw")
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, rancher.ErrLoginFailed) {
		t.Fatalf("a missing provider is not a wrong password: %v", err)
	}
	if !strings.Contains(err.Error(), "not found") || !strings.Contains(err.Error(), "--auth-provider-type openLdap --auth-provider-id openldap") {
		t.Fatalf("expected original error plus provider hint, got:\n%v", err)
	}
}

func TestCreateTokenAutoUsesDetectedLoginPath(t *testing.T) {
	var loginPath string
	srv := providerServer(t, `{"data":[{"id":"openldap","type":"openLdapProvider"}]}`, func(w http.ResponseWriter, r *http.Request) {
		loginPath = r.URL.Path
		_, _ = w.Write([]byte(`{"token":"session"}`))
	})
	// CreateAPIToken goes to /v3/tokens on the same server; providerServer answers 404 there,
	// which is enough to assert the login path that was used.
	_, _ = createTokenViaAuthLogin(context.Background(), srv.URL, false, autoLogin(), "pw")
	if loginPath != "/v3-public/openLdapProviders/openldap" {
		t.Fatalf("expected detected login path, got %q", loginPath)
	}
}
