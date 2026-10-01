package core

import "testing"

func TestValidateActor(t *testing.T) {
	for _, actor := range []string{"human:Alice Smith", "agent:case-officer", "system:session-end"} {
		if err := ValidateActor(actor); err != nil {
			t.Errorf("ValidateActor(%q) = %v", actor, err)
		}
	}
	for _, actor := range []string{"agent:../admin", "agent:Not A Slug", "unknown:alice", "human:bad\nentry"} {
		if err := ValidateActor(actor); err == nil {
			t.Errorf("ValidateActor(%q) unexpectedly succeeded", actor)
		}
	}
}
