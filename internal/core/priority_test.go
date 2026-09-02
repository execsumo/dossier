package core

import "testing"

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
