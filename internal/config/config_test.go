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
