package core

import (
	"strings"
	"testing"
)

func TestCollapseProvenance(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
		not  []string
	}{
		{"no refs untouched", "plain text", []string{"plain text"}, []string{"Sources", "---"}},
		{"numbered by first appearance", "a [src:art_1#L1-L2] b [src:art_2] c [src:art_1#L1-L2]",
			[]string{"a ¹ b ² c ¹", "¹ `art_1 L1-L2`", "² `art_2`"}, []string{"[src:"}},
		{"adjacent refs separated", "x [src:art_1][src:art_2]", []string{"x ¹ ²"}, nil},
		{"fenced code untouched", "```\n[src:art_1]\n```", []string{"[src:art_1]"}, []string{"Sources", "¹"}},
		{"multi digit", strings.Repeat("[src:a] ", 1) + "[src:b] [src:c] [src:d] [src:e] [src:f] [src:g] [src:h] [src:i] [src:j] [src:k]",
			[]string{"¹¹ `k`"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CollapseProvenance(tt.in)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in:\n%s", w, got)
				}
			}
			for _, n := range tt.not {
				if strings.Contains(got, n) {
					t.Errorf("unexpected %q in:\n%s", n, got)
				}
			}
		})
	}
}
