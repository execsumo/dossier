package cli

import (
	"testing"
)

func TestNormalizeLineFlag(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"10-20", "L10-L20"},
		{"L10-L20", "L10-L20"},
		{"#L10-L20", "L10-L20"},
		{"42", "L42"},
		{"L42", "L42"},
		{" 10 - 20 ", "L10-L20"},
	}
	for _, tt := range tests {
		if got := normalizeLineFlag(tt.in); got != tt.want {
			t.Errorf("normalizeLineFlag(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
