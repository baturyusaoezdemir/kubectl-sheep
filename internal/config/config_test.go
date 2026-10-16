package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateURL(t *testing.T) {
	tests := []struct {
		input string
		ok    bool
	}{
		{"https://rancher.example.com", true},
		{"http://localhost:8080", true},
		{"not-a-url", false},
		{"", false},
		{"https://", false},
	}

	for _, tt := range tests {
		err := ValidateURL(tt.input)
		if tt.ok && err != nil {
			t.Errorf("ValidateURL(%q) unexpected error: %v", tt.input, err)
		}
		if !tt.ok && err == nil {
			t.Errorf("ValidateURL(%q) expected error", tt.input)
		}
	}
}

func TestAddRemoveInstance(t *testing.T) {
	cfg := &Config{}

	inst := Instance{
		Name:    "prod",
		URL:     "https://rancher.prod.example.com",
		Storage: StorageEncrypted,
	}
	if err := cfg.AddInstance(inst); err != nil {
		t.Fatalf("AddInstance: %v", err)
	}

	if err := cfg.ValidateName("prod"); err == nil {
		t.Fatal("expected duplicate name error")
	}

	if err := cfg.RemoveInstance("prod"); err != nil {
		t.Fatalf("RemoveInstance: %v", err)
	}
	if len(cfg.Instances) != 0 {
		t.Fatalf("expected empty instances, got %d", len(cfg.Instances))
	}
}

func TestSaveLoad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &Config{
		Instances: []Instance{
			{
				Name:               "dev",
				URL:                "https://rancher.dev.example.com",
				InsecureSkipVerify: true,
				Storage:            StoragePlaintext,
			},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path, err := InstancesPath()
	if err != nil {
		t.Fatalf("InstancesPath: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected mode 0600, got %o", info.Mode().Perm())
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Instances) != 1 || loaded.Instances[0].Name != "dev" {
		t.Fatalf("unexpected loaded config: %+v", loaded.Instances)
	}

	dir := filepath.Join(home, ".config", configDirName)
	info, err = os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat dir: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("expected dir mode 0700, got %o", info.Mode().Perm())
	}
}

func TestAuthLoginRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &Config{
		Instances: []Instance{
			{Name: "manual", URL: "https://a.example.com", Storage: StoragePlaintext},
			{
				Name:    "ad",
				URL:     "https://b.example.com",
				Storage: StorageEncrypted,
				Auth:    &AuthLogin{ProviderType: "activeDirectory", ProviderID: "activeDirectory", Username: "alice"},
			},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path, _ := InstancesPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(raw), "auth:") != 1 {
		t.Fatalf("auth block should only be written when set:\n%s", raw)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	manual, _ := loaded.Find("manual")
	if manual.Auth != nil {
		t.Fatalf("expected no auth login for manual instance, got %+v", manual.Auth)
	}
	ad, _ := loaded.Find("ad")
	if ad.Auth == nil || ad.Auth.Username != "alice" || ad.Auth.ProviderType != "activeDirectory" {
		t.Fatalf("unexpected auth login: %+v", ad.Auth)
	}
	if ad.Auth.String() != "activeDirectory (alice)" {
		t.Fatalf("unexpected String(): %q", ad.Auth.String())
	}
	if got := (&AuthLogin{ProviderType: "ldap", ProviderID: "openldap", Username: "bob"}).String(); got != "ldap/openldap (bob)" {
		t.Fatalf("unexpected String(): %q", got)
	}
}

func TestAuthLoginValidate(t *testing.T) {
	var nilLogin *AuthLogin
	if err := nilLogin.Validate(); err == nil {
		t.Fatal("expected error for nil login")
	}
	if err := (&AuthLogin{ProviderType: "ldap", ProviderID: "openldap"}).Validate(); err == nil {
		t.Fatal("expected error for missing username")
	}
	if err := (&AuthLogin{ProviderType: "ldap", ProviderID: "openldap", Username: "bob"}).Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
