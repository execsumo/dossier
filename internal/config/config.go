package config

import (
	"bytes"
	"dossier/internal/core"
	"fmt"
	"os"
	osuser "os/user"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// TeamConfig holds configuration for the team sync remote.
type TeamConfig struct {
	Remote string `yaml:"remote,omitempty"`
	Branch string `yaml:"branch,omitempty"`
}

// Config represents the schema of ~/.dossier/config.yaml.
type Config struct {
	DossierHome string     `yaml:"dossier_home"`
	Author      string     `yaml:"author"`
	Interfaces  []string   `yaml:"interfaces"`
	Leads       []string   `yaml:"leads"`
	Team        TeamConfig `yaml:"team,omitempty"`
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
		Author:      author,
		Interfaces:  core.DefaultDiscussionInterfaces(),
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
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(cfg); err != nil {
		return nil, err
	}
	if err := cfg.validateValues(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Save marshals and writes the configuration to a YAML file.
func (c *Config) Save(path string) error {
	if err := c.validateValues(); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (c *Config) validateValues() error {
	for _, vocabulary := range []struct {
		label  string
		values []string
	}{
		{label: "interfaces", values: c.Interfaces},
		{label: "leads", values: c.Leads},
	} {
		if err := validateVocabulary(vocabulary.label, vocabulary.values); err != nil {
			return err
		}
	}
	return nil
}

func validateVocabulary(label string, values []string) error {
	seen := make(map[string]bool, len(values))
	var whitespaceValue string
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return fmt.Errorf("%s must not contain blank values", label)
		}
		if seen[trimmed] {
			return fmt.Errorf("%s contains duplicate value %q (ignoring surrounding whitespace)", label, trimmed)
		}
		seen[trimmed] = true
		if trimmed != value && whitespaceValue == "" {
			whitespaceValue = value
		}
	}
	if whitespaceValue != "" {
		return fmt.Errorf("%s value %q must not contain leading or trailing whitespace", label, whitespaceValue)
	}
	return nil
}

// ToCoreConfig maps the configuration to the core service Config.
func (c *Config) ToCoreConfig() core.Config {
	return core.Config{
		DossierHome: c.DossierHome,
		Author:      c.Author,
		Interfaces:  append([]string{}, c.Interfaces...),
		Leads:       append([]string{}, c.Leads...),
	}
}
