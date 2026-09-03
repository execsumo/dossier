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
	cfg := Default()
	cfg.Leads = []string{"Alice", "Alice"}
	if err := cfg.Save(filepath.Join(t.TempDir(), "config.yaml")); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("Save() error = %v, want duplicate-value error", err)
	}

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("interfaces:\n  - '  '\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "blank") {
		t.Fatalf("Load() error = %v, want blank-value error", err)
	}
}
