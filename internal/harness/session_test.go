package harness

import "testing"

func TestResolveSessionID(t *testing.T) {
	tests := []struct {
		name         string
		explicit     string
		claudeEnv    string
		piEnv        string
		dossierEnv   string
		allowDefault bool
		want         string
		wantErr      bool
	}{
		{"explicit wins over env", "explicit-1", "claude-1", "", "dossier-1", false, "explicit-1", false},
		{"claude env beats pi and dossier env", "", "claude-1", "pi-1", "dossier-1", false, "claude-1", false},
		{"pi env beats dossier env", "", "", "pi-1", "dossier-1", false, "pi-1", false},
		{"dossier env when no harness env", "", "", "", "dossier-1", false, "dossier-1", false},
		{"default when allowed and nothing set", "", "", "", "", true, DefaultSessionID, false},
		{"error when not allowed and nothing set", "", "", "", "", false, "", true},
		{"explicit still wins when default allowed", "explicit-2", "", "", "", true, "explicit-2", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Empty value behaves as unset for our != "" checks, and t.Setenv restores after.
			t.Setenv("CLAUDE_CODE_SESSION_ID", tt.claudeEnv)
			t.Setenv("PI_SESSION_ID", tt.piEnv)
			t.Setenv("DOSSIER_SESSION", tt.dossierEnv)

			got, err := ResolveSessionID(tt.explicit, tt.allowDefault)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (got=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ResolveSessionID(%q, %v) = %q, want %q", tt.explicit, tt.allowDefault, got, tt.want)
			}
		})
	}
}
