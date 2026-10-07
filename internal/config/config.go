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

// EvalConfig is the knob for automatic session evals (ADR 0016). Each eval
// spends three model calls, so it can be switched off.
type EvalConfig struct {
	// Enabled turns automatic evals on. A pointer so an absent key keeps the
	// default rather than reading as false.
	Enabled *bool `yaml:"enabled,omitempty"`
	// Model is passed to the evaluator for every call.
	Model string `yaml:"model,omitempty"`
	// Effort is the reasoning effort for every eval call (low, medium, high,
	// xhigh, max). Empty uses the model's default. Models without effort
	// support (Haiku) ignore it.
	Effort string `yaml:"effort,omitempty"`
}

// DefaultEvalModel is the model automatic evals use unless configured.
const DefaultEvalModel = "haiku"

// defaultEvalEnabled is the out-of-the-box setting. On while version-by-version
// tracking is being established; flip here (or per machine in config.yaml)
// once the cost is no longer worth it.
const defaultEvalEnabled = true

// EvalEnabled resolves the knob against its default.
func (e EvalConfig) EvalEnabled() bool {
	if e.Enabled == nil {
		return defaultEvalEnabled
	}
	return *e.Enabled
}

// EvalModel resolves the model against its default.
func (e EvalConfig) EvalModel() string {
	if m := strings.TrimSpace(e.Model); m != "" {
		return m
	}
	return DefaultEvalModel
}

// Config represents the canonical schema of ~/.dossier/config.yaml.
type Config struct {
	SchemaVersion int        `yaml:"schema_version,omitempty"`
	DossierHome   string     `yaml:"dossier_home"`
	Author        string     `yaml:"author"`
	DisplayName   string     `yaml:"display_name,omitempty"`
	OpenWith      string     `yaml:"open_with,omitempty"`
	Interfaces    []string   `yaml:"interfaces"`
	Leads         []string   `yaml:"leads"`
	Team          TeamConfig `yaml:"team,omitempty"`
	TokenLimit    int        `yaml:"token_limit,omitempty"`
	// RepoRoots are folders searched (one level deep) for checkouts of a
	// Dossier's repos when this machine has not learned their location yet
	// (ADR 0015). Machine-local, like the rest of this file.
	RepoRoots []string `yaml:"repo_roots,omitempty"`
	// Eval is the automatic session eval knob (ADR 0016).
	Eval EvalConfig `yaml:"eval,omitempty"`
}

// configFile is the strict read schema. TokenTarget and SchemaVersion are
// accepted for backward compatibility with pre-simplification configs.
type configFile struct {
	DossierHome   string     `yaml:"dossier_home"`
	Author        string     `yaml:"author"`
	DisplayName   string     `yaml:"display_name,omitempty"`
	OpenWith      string     `yaml:"open_with,omitempty"`
	Interfaces    []string   `yaml:"interfaces"`
	Leads         []string   `yaml:"leads"`
	Team          TeamConfig `yaml:"team,omitempty"`
	TokenLimit    *int       `yaml:"token_limit,omitempty"`
	TokenTarget   *int       `yaml:"token_target,omitempty"`
	SchemaVersion int        `yaml:"schema_version,omitempty"`
	RepoRoots     []string   `yaml:"repo_roots,omitempty"`
	Eval          EvalConfig `yaml:"eval,omitempty"`
}

// CurrentSchemaVersion is the latest on-disk config schema.
const CurrentSchemaVersion = 3

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
		SchemaVersion: CurrentSchemaVersion,
		DossierHome:   homePath,
		Author:        core.NormalizeUsername(author),
		OpenWith:      "claude-code",
		Interfaces:    core.DefaultDiscussionInterfaces(),
		Leads:         []string{},
		TokenLimit:    core.DefaultTokenLimit,
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
	wire := configFile{
		DossierHome: cfg.DossierHome,
		Author:      cfg.Author,
		DisplayName: cfg.DisplayName,
		OpenWith:    cfg.OpenWith,
		Interfaces:  cfg.Interfaces,
		Leads:       cfg.Leads,
		Team:        cfg.Team,
	}
	data, _, err = migrateYAML(data)
	if err != nil {
		return nil, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&wire); err != nil {
		return nil, err
	}
	cfg.SchemaVersion = CurrentSchemaVersion
	cfg.DossierHome = wire.DossierHome
	cfg.Author = strings.TrimSpace(wire.Author)
	cfg.DisplayName = strings.TrimSpace(wire.DisplayName)
	cfg.OpenWith = wire.OpenWith
	cfg.Interfaces = wire.Interfaces
	cfg.Leads = wire.Leads
	cfg.Team = wire.Team
	cfg.RepoRoots = wire.RepoRoots
	cfg.Eval = wire.Eval
	if wire.TokenLimit != nil {
		cfg.TokenLimit = *wire.TokenLimit
	} else if wire.TokenTarget != nil {
		cfg.TokenLimit = *wire.TokenTarget
	}
	if err := cfg.validateValues(); err != nil {
		return nil, err
	}
	return cfg, nil
}

const defaultConfigHelp = `# Dossier configuration. Edit the lists below as needed.
# Interfaces are exact, case-sensitive values used to categorize Dossiers.
# Leads are optional: leave the list empty for free-form lead names, or add
# names to restrict lead assignments to that vocabulary.
# Example leads:
#   - Alice
#   - Bob
#
# Automatic session evals score, after each saved session, how well the
# Distilled State preserves what the session established (3 model calls per
# session; see "dossier stats"). They are on by default. To turn them off or
# change the model:
#   eval:
#     enabled: false
#     model: haiku
#     effort: medium   # low|medium|high|xhigh|max; Haiku ignores effort
`

// Migrate upgrades an existing config in place, keeping a .bak copy of its
// original contents. It is safe to rerun after a successful migration.
func Migrate(path string) error {
	if _, err := Load(path); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	migrated, changed, err := migrateYAML(data)
	if err != nil || !changed {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	backup := path + ".bak"
	if err := writeBackup(backup, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("backup config before migration: %w", err)
	}
	return os.WriteFile(path, migrated, info.Mode().Perm())
}

func writeBackup(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if os.IsExist(err) {
		existing, readErr := os.ReadFile(path)
		if readErr == nil && bytes.Equal(existing, data) {
			return nil
		}
		return fmt.Errorf("%s already exists", path)
	}
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func migrateYAML(data []byte) ([]byte, bool, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, false, err
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, false, fmt.Errorf("config must be a YAML mapping")
	}
	root := doc.Content[0]
	version := 0
	versionNode := -1
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "schema_version" {
			versionNode = i + 1
			if err := root.Content[versionNode].Decode(&version); err != nil {
				return nil, false, fmt.Errorf("invalid schema_version: %w", err)
			}
			break
		}
	}
	if version < 0 {
		return nil, false, fmt.Errorf("config schema version must not be negative")
	}
	if version > CurrentSchemaVersion {
		return nil, false, fmt.Errorf("config schema version %d is newer than supported version %d", version, CurrentSchemaVersion)
	}
	if version == CurrentSchemaVersion {
		return data, false, nil
	}
	for version < CurrentSchemaVersion {
		switch version {
		case 0, 1:
			// These versions have no field transformations.
		case 2:
			migrateTokenTarget(root)
		default:
			return nil, false, fmt.Errorf("no migration from config schema version %d", version)
		}
		version++
	}
	if versionNode < 0 {
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "schema_version"}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprint(CurrentSchemaVersion)})
	} else {
		root.Content[versionNode].Tag = "!!int"
		root.Content[versionNode].Value = fmt.Sprint(CurrentSchemaVersion)
	}
	migrated, err := yaml.Marshal(&doc)
	return migrated, true, err
}

func migrateTokenTarget(root *yaml.Node) {
	var tokenLimit *yaml.Node
	var tokenTarget *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		switch root.Content[i].Value {
		case "token_limit":
			tokenLimit = root.Content[i+1]
		case "token_target":
			tokenTarget = root.Content[i+1]
		}
	}
	if tokenTarget == nil {
		return
	}
	if tokenLimit == nil {
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "token_limit"}, tokenTarget)
	}
	content := root.Content[:0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "token_target" {
			content = append(content, root.Content[i], root.Content[i+1])
		}
	}
	root.Content = content
}

// Save marshals and writes the configuration to a YAML file.
func (c *Config) Save(path string) error {
	return c.save(path, false)
}

// SaveDefault marshals and writes a newly generated configuration with inline
// guidance for the user-editable interface and lead lists.
func (c *Config) SaveDefault(path string) error {
	return c.save(path, true)
}

func (c *Config) save(path string, includeHelp bool) error {
	if err := c.validateValues(); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	if includeHelp {
		data = append([]byte(defaultConfigHelp), data...)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (c *Config) validateValues() error {
	if c.TokenLimit < 0 {
		return fmt.Errorf("token_limit must not be negative")
	}
	if effort := strings.ToLower(strings.TrimSpace(c.Eval.Effort)); effort != "" {
		valid := false
		for _, level := range core.EvalEffortLevels {
			valid = valid || effort == level
		}
		if !valid {
			return fmt.Errorf("eval.effort %q is not one of %s", c.Eval.Effort, strings.Join(core.EvalEffortLevels, ", "))
		}
	}
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
		DisplayName: c.DisplayName,
		Interfaces:  append([]string{}, c.Interfaces...),
		Leads:       append([]string{}, c.Leads...),
		TokenLimit:  c.TokenLimit,
		TeamRemote:  c.Team.Remote,
		Eval: core.EvalConfig{
			Enabled: c.Eval.EvalEnabled(),
			Model:   c.Eval.EvalModel(),
			Effort:  strings.ToLower(strings.TrimSpace(c.Eval.Effort)),
		},
	}
}
