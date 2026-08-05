package config

import (
	"dossier/internal/core"
	"os"
	osuser "os/user"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// TeamConfig holds configuration for the team sync remote.
type TeamConfig struct {
	Remote string `yaml:"remote,omitempty"`
	Branch string `yaml:"branch,omitempty"`
}

// Config represents the schema of ~/.dossier/config.yaml.
type Config struct {
	DossierHome string `yaml:"dossier_home"`
	TokenTarget int    `yaml:"token_target"`
	Author      string `yaml:"author"`
	// SchemaVersion records the frontmatter schema the store was last migrated to.
	// It is intentionally left at its zero value in Default so that a config file
	// written by an older build (which lacks the key) is detected as stale and
	// triggers the one-time migration sweep on the next launch.
	SchemaVersion int        `yaml:"schema_version"`
	Team          TeamConfig `yaml:"team,omitempty"`
}

// Default returns the default configuration with standard paths.
func Default() *Config {
	homePath := ""
	if envHome := os.Getenv("DOSSIER_HOME"); envHome != "" {
		homePath = envHome
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			homePath = ".dossier"
		} else {
			homePath = filepath.Join(home, ".dossier")
		}
	}

	var author string
	if u, err := osuser.Current(); err == nil && u.Username != "" {
		author = u.Username
	} else if envUser := os.Getenv("USER"); envUser != "" {
		author = envUser
	} else {
		author = "unknown"
	}

	return &Config{
		DossierHome: homePath,
		TokenTarget: 100000,
		Author:      author,
	}
}

// Load loads config from a YAML file, falling back to defaults if not found.
func Load(path string) (*Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Save marshals and writes the configuration to a YAML file.
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// ToCoreConfig maps the configuration to the core service Config.
func (c *Config) ToCoreConfig() core.Config {
	return core.Config{
		DossierHome: c.DossierHome,
		TokenTarget: c.TokenTarget,
		Author:      c.Author,
	}
}
