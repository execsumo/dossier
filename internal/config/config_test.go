package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadValueLists(t *testing.T) {
	t.Run("missing file uses interface defaults", func(t *testing.T) {
		cfg, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if len(cfg.Interfaces) != 7 {
			t.Fatalf("Interfaces = %v, want seven defaults", cfg.Interfaces)
		}
		if len(cfg.Leads) != 0 {
			t.Fatalf("Leads = %v, want empty", cfg.Leads)
		}
	})

	t.Run("old config inherits defaults", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte("dossier_home: /tmp/dossiers\nauthor: test\n"), 0644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(cfg.Interfaces) != 7 {
			t.Fatalf("Interfaces = %v, want seven defaults", cfg.Interfaces)
		}
	})

	t.Run("custom and explicitly empty values round trip", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		cfg := Default()
		cfg.Interfaces = []string{}
		cfg.Leads = []string{"Alice", "Bob"}
		if err := cfg.Save(path); err != nil {
			t.Fatal(err)
		}
		loaded, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Interfaces == nil || len(loaded.Interfaces) != 0 {
			t.Fatalf("Interfaces = %#v, want explicit empty list", loaded.Interfaces)
		}
		if !reflect.DeepEqual(loaded.Leads, cfg.Leads) {
			t.Fatalf("Leads = %v, want %v", loaded.Leads, cfg.Leads)
		}
		coreCfg := loaded.ToCoreConfig()
		loaded.Leads[0] = "mutated"
		if coreCfg.Leads[0] != "Alice" {
			t.Fatal("ToCoreConfig aliased lead storage")
		}
	})
}

func TestLoadLegacyConfigAndSaveCanonical(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	legacy := `dossier_home: /tmp/dossiers
token_target: 100000
author: legacy-user
schema_version: 2
team:
  remote: https://example.test/team.git
  branch: main
`
	if err := os.WriteFile(path, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() rejected legacy config: %v", err)
	}
	if cfg.DossierHome != "/tmp/dossiers" || cfg.Author != "legacy-user" {
		t.Fatalf("legacy values not loaded: %+v", cfg)
	}
	if len(cfg.Interfaces) != 7 {
		t.Fatalf("legacy config did not inherit interface defaults: %v", cfg.Interfaces)
	}
	unchanged, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != legacy {
		t.Fatal("Load() eagerly rewrote the legacy config")
	}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(written), "token_target:") || strings.Contains(string(written), "schema_version:") {
		t.Fatalf("canonical save re-emitted compatibility fields:\n%s", written)
	}
}

func TestLoadRejectsUnknownConfigField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("dossier_home: /tmp/dossiers\nauthor: test\nunknown_setting: true\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "unknown_setting") {
		t.Fatalf("Load() error = %v, want strict unknown-field error", err)
	}
}

func TestConfigRejectsInvalidValueLists(t *testing.T) {
	tests := []struct {
		name string
		set  func(*Config)
		want []string
	}{
		{
			name: "interface leading whitespace",
			set:  func(cfg *Config) { cfg.Interfaces = []string{" Planning"} },
			want: []string{"interfaces", "leading or trailing whitespace"},
		},
		{
			name: "interface trailing whitespace",
			set:  func(cfg *Config) { cfg.Interfaces = []string{"Planning\t"} },
			want: []string{"interfaces", "leading or trailing whitespace"},
		},
		{
			name: "interface whitespace-equivalent duplicate",
			set:  func(cfg *Config) { cfg.Interfaces = []string{"Planning", "\tPlanning "} },
			want: []string{"interfaces", "duplicate"},
		},
		{
			name: "lead leading whitespace",
			set:  func(cfg *Config) { cfg.Leads = []string{" Alice"} },
			want: []string{"leads", "leading or trailing whitespace"},
		},
		{
			name: "lead trailing whitespace",
			set:  func(cfg *Config) { cfg.Leads = []string{"Alice\t"} },
			want: []string{"leads", "leading or trailing whitespace"},
		},
		{
			name: "lead whitespace-equivalent duplicate",
			set:  func(cfg *Config) { cfg.Leads = []string{"Alice", " Alice\t"} },
			want: []string{"leads", "duplicate"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.set(cfg)
			err := cfg.Save(filepath.Join(t.TempDir(), "config.yaml"))
			if err == nil {
				t.Fatal("Save() succeeded, want validation error")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("Save() error = %v, want it to contain %q", err, want)
				}
			}
		})
	}

	t.Run("load rejects blank interface", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte("interfaces:\n  - '  '\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "blank") {
			t.Fatalf("Load() error = %v, want blank-value error", err)
		}
	})

	t.Run("load rejects whitespace-equivalent lead duplicates", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte("leads:\n  - Alice\n  - ' Alice '\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "leads contains duplicate") {
			t.Fatalf("Load() error = %v, want whitespace-equivalent duplicate error", err)
		}
	})
}

func TestTokenLimitConfig(t *testing.T) {
	t.Run("default is 100000", func(t *testing.T) {
		cfg := Default()
		if cfg.TokenLimit != 100000 {
			t.Fatalf("Default().TokenLimit = %d, want 100000", cfg.TokenLimit)
		}
		coreCfg := cfg.ToCoreConfig()
		if coreCfg.TokenLimit != 100000 {
			t.Fatalf("ToCoreConfig().TokenLimit = %d, want 100000", coreCfg.TokenLimit)
		}
	})

	t.Run("loads custom token_limit and round trips", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		content := "dossier_home: /tmp/dossiers\nauthor: user\ntoken_limit: 50000\n"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if cfg.TokenLimit != 50000 {
			t.Fatalf("cfg.TokenLimit = %d, want 50000", cfg.TokenLimit)
		}
		coreCfg := cfg.ToCoreConfig()
		if coreCfg.TokenLimit != 50000 {
			t.Fatalf("coreCfg.TokenLimit = %d, want 50000", coreCfg.TokenLimit)
		}

		savePath := filepath.Join(t.TempDir(), "saved.yaml")
		if err := cfg.Save(savePath); err != nil {
			t.Fatalf("Save() error: %v", err)
		}
		data, err := os.ReadFile(savePath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "token_limit: 50000") {
			t.Fatalf("saved config missing token_limit: 50000:\n%s", string(data))
		}
	})

	t.Run("accepts legacy token_target", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		content := "dossier_home: /tmp/dossiers\nauthor: user\ntoken_target: 75000\n"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if cfg.TokenLimit != 75000 {
			t.Fatalf("cfg.TokenLimit = %d, want 75000", cfg.TokenLimit)
		}
	})

	t.Run("token_limit takes precedence over token_target", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		content := "dossier_home: /tmp/dossiers\nauthor: user\ntoken_limit: 60000\ntoken_target: 80000\n"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if cfg.TokenLimit != 60000 {
			t.Fatalf("cfg.TokenLimit = %d, want 60000", cfg.TokenLimit)
		}
	})

	t.Run("rejects negative token_limit", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		content := "dossier_home: /tmp/dossiers\nauthor: user\ntoken_limit: -10\n"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "token_limit must not be negative") {
			t.Fatalf("Load() error = %v, want negative token_limit error", err)
		}

		cfg := Default()
		cfg.TokenLimit = -5
		if err := cfg.Save(filepath.Join(t.TempDir(), "out.yaml")); err == nil || !strings.Contains(err.Error(), "token_limit must not be negative") {
			t.Fatalf("Save() error = %v, want negative token_limit error", err)
		}
	})
}
