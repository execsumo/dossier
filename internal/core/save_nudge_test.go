package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSaveNudge(t *testing.T) {
	base := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	at := func(minutes ...int) []time.Time {
		var out []time.Time
		for _, m := range minutes {
			out = append(out, base.Add(time.Duration(m)*time.Minute))
		}
		return out
	}
	tests := []struct {
		name      string
		turns     int // config SaveNudgeTurns
		unbound   bool
		active    bool
		nudgedAt  int // minutes after base; -1 for never
		activity  SessionActivity
		wantNudge bool
	}{
		{name: "three turns of work since start", turns: 3, nudgedAt: -1,
			activity: SessionActivity{Prompts: at(1, 2, 3), ToolUses: at(1)}, wantNudge: true},
		{name: "below threshold", turns: 3, nudgedAt: -1,
			activity: SessionActivity{Prompts: at(1, 2), ToolUses: at(1, 2)}},
		{name: "turns without tool work", turns: 3, nudgedAt: -1,
			activity: SessionActivity{Prompts: at(1, 2, 3)}},
		{name: "save resets the count", turns: 3, nudgedAt: -1,
			activity: SessionActivity{Prompts: at(1, 2, 3, 5, 6), ToolUses: at(1, 5), Saves: at(4)}},
		{name: "enough turns after a save", turns: 3, nudgedAt: -1,
			activity: SessionActivity{Prompts: at(1, 5, 6, 7), ToolUses: at(6), Saves: at(4)}, wantNudge: true},
		{name: "previous nudge resets the count", turns: 3, nudgedAt: 4,
			activity: SessionActivity{Prompts: at(1, 2, 3, 5, 6), ToolUses: at(5)}},
		{name: "stop hook already active", turns: 3, nudgedAt: -1, active: true,
			activity: SessionActivity{Prompts: at(1, 2, 3), ToolUses: at(1)}},
		{name: "disabled by config", turns: 0, nudgedAt: -1,
			activity: SessionActivity{Prompts: at(1, 2, 3), ToolUses: at(1)}},
		{name: "session not bound to a dossier", turns: 3, nudgedAt: -1, unbound: true,
			activity: SessionActivity{Prompts: at(1, 2, 3), ToolUses: at(1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newLocalFakeStore()
			now := base.Add(time.Hour)
			svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now},
				Config{DossierHome: "/tmp/dossier-test", SaveNudgeTurns: tt.turns}, nil)
			store.dossiers["dos_1"] = &Dossier{Frontmatter: Frontmatter{ID: "dos_1", Name: "Billing lock", Slug: "billing-lock"}}
			binding := &SessionBinding{SessionBindingID: "sess_1", DossierID: "dos_1"}
			if tt.unbound {
				binding.DossierID = ""
			}
			if tt.nudgedAt >= 0 {
				binding.SaveNudgedAt = base.Add(time.Duration(tt.nudgedAt) * time.Minute)
			}
			store.sessions["sess_1"] = binding

			reason, err := svc.SaveNudge(context.Background(), SaveNudgeReq{
				SessionID: "sess_1", StopHookActive: tt.active, Activity: tt.activity,
			})
			if err != nil {
				t.Fatalf("SaveNudge() error = %v", err)
			}
			if got := reason != ""; got != tt.wantNudge {
				t.Fatalf("nudged = %v, want %v (reason %q)", got, tt.wantNudge, reason)
			}
			if !tt.wantNudge {
				return
			}
			for _, want := range []string{"Billing lock", "dossier_save", "base_revision", "memory files do not replace the Dossier"} {
				if !strings.Contains(reason, want) {
					t.Errorf("reason missing %q: %s", want, reason)
				}
			}
			if got := store.sessions["sess_1"].SaveNudgedAt; !got.Equal(now) {
				t.Errorf("SaveNudgedAt = %v, want %v so the nudge does not repeat", got, now)
			}
		})
	}
}
