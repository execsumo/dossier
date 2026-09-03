package core

import "testing"

func TestPriorityFromLegacyMatrix(t *testing.T) {
	tests := []struct {
		name       string
		importance string
		urgency    string
		want       Priority
	}{
		{name: "high high", importance: "high", urgency: "high", want: PriorityMax},
		{name: "high low", importance: "high", urgency: "low", want: PriorityHigh},
		{name: "low high", importance: "low", urgency: "high", want: PriorityMedium},
		{name: "low low", importance: "low", urgency: "low", want: PriorityLow},
		{name: "missing importance normalizes high", urgency: "low", want: PriorityHigh},
		{name: "missing urgency normalizes high", importance: "low", want: PriorityMedium},
		{name: "both missing normalize high", want: PriorityMax},
		{name: "unknown dimensions normalize high", importance: "medium", urgency: "eventual", want: PriorityMax},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PriorityFromLegacyMatrix(tt.importance, tt.urgency); got != tt.want {
				t.Fatalf("PriorityFromLegacyMatrix(%q, %q) = %q, want %q", tt.importance, tt.urgency, got, tt.want)
			}
		})
	}
}

func TestPriorityOrdering(t *testing.T) {
	ordered := []Priority{PriorityMax, PriorityHigh, PriorityMedium, PriorityLow}
	for i, higher := range ordered {
		for j, lower := range ordered {
			if i < j && !priorityBefore(higher, lower) {
				t.Errorf("expected %q before %q", higher, lower)
			}
			if i > j && priorityBefore(higher, lower) {
				t.Errorf("expected %q after %q", higher, lower)
			}
		}
	}
}
