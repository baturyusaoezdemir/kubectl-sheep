package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	StoragePlaintext  = "plaintext"
	StorageEncrypted  = "encrypted"
	configDirName     = "kubectl-sheep"
	instancesFileName = "instances.yaml"
)

// Instance holds non-secret configuration for a Rancher instance.
type Instance struct {
	Name               string     `yaml:"name"`
	URL                string     `yaml:"url"`
	InsecureSkipVerify bool       `yaml:"insecureSkipVerify"`
	Storage            string     `yaml:"storage"`
	Auth               *AuthLogin `yaml:"auth,omitempty"`
}

// AuthLogin describes how to obtain a Rancher API token through a Rancher auth
// provider (for example Active Directory or OpenLDAP). The password is never
// stored; it is prompted for whenever a token has to be (re)created.
type AuthLogin struct {
	ProviderType string `yaml:"providerType"`
	ProviderID   string `yaml:"providerId"`
	Username     string `yaml:"username"`
}

// Key identifies a login so that one password prompt can serve every instance
// sharing the same provider and username.
func (a *AuthLogin) Key() string {
	if a == nil {
		return ""
	}
	return a.ProviderType + "/" + a.ProviderID + "/" + a.Username
}

// String returns a short human-readable description, e.g. "activeDirectory (alice)".
func (a *AuthLogin) String() string {
	if a == nil {
		return ""
	}
	label := a.ProviderType
	if a.ProviderID != "" && a.ProviderID != a.ProviderType {
		label += "/" + a.ProviderID
	}
	if a.Username != "" {
		label += " (" + a.Username + ")"
	}
	return label
}

// Validate checks that all AuthLogin fields are set.
func (a *AuthLogin) Validate() error {
	if a == nil {
		return fmt.Errorf("auth login must not be nil")
	}
	if strings.TrimSpace(a.ProviderType) == "" {
		return fmt.Errorf("auth provider type must not be empty")
	}
	if strings.TrimSpace(a.ProviderID) == "" {
		return fmt.Errorf("auth provider ID must not be empty")
	}
	if strings.TrimSpace(a.Username) == "" {
		return fmt.Errorf("auth username must not be empty")
	}
	return nil
}

// Config is the top-level instances configuration file.
type Config struct {
	Instances []Instance `yaml:"instances"`
}

// ConfigDir returns ~/.config/kubectl-sheep.
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", configDirName), nil
}

// InstancesPath returns the path to instances.yaml.
func InstancesPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, instancesFileName), nil
}

// Load reads instances.yaml, returning an empty config if the file does not exist.
func Load() (*Config, error) {
	path, err := InstancesPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("read instances config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse instances config: %w", err)
	}
	return &cfg, nil
}

// Save writes instances.yaml, creating parent directories as needed.
func (c *Config) Save() error {
	path, err := InstancesPath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal instances config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write instances config: %w", err)
	}
	return nil
}

// Find returns the instance with the given name.
func (c *Config) Find(name string) (*Instance, error) {
	for i := range c.Instances {
		if c.Instances[i].Name == name {
			return &c.Instances[i], nil
		}
	}
	return nil, fmt.Errorf("instance %q not found", name)
}

// ValidateName checks that a name is non-empty and not already used.
func (c *Config) ValidateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("instance name must not be empty")
	}
	for _, inst := range c.Instances {
		if inst.Name == name {
			return fmt.Errorf("instance %q already exists", name)
		}
	}
	return nil
}

// ValidateURL checks that raw is a parseable URL with scheme and host.
func ValidateURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("URL must not be empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme == "" {
		return fmt.Errorf("URL must include a scheme (e.g. https://)")
	}
	if u.Host == "" {
		return fmt.Errorf("URL must include a host")
	}
	return nil
}

// ValidateStorage checks that storage is plaintext or encrypted.
func ValidateStorage(storage string) error {
	switch storage {
	case StoragePlaintext, StorageEncrypted:
		return nil
	default:
		return fmt.Errorf("storage must be %q or %q", StoragePlaintext, StorageEncrypted)
	}
}

// AddInstance appends a new instance after validation.
func (c *Config) AddInstance(inst Instance) error {
	if err := c.ValidateName(inst.Name); err != nil {
		return err
	}
	if err := ValidateURL(inst.URL); err != nil {
		return err
	}
	if err := ValidateStorage(inst.Storage); err != nil {
		return err
	}
	c.Instances = append(c.Instances, inst)
	return nil
}

// RemoveInstance deletes the instance with the given name.
func (c *Config) RemoveInstance(name string) error {
	for i, inst := range c.Instances {
		if inst.Name == name {
			c.Instances = append(c.Instances[:i], c.Instances[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("instance %q not found", name)
}
