package harness

import (
	"reflect"
	"testing"
)

func TestInHerdr(t *testing.T) {
	for val, want := range map[string]bool{"1": true, "": false, "0": false} {
		t.Setenv("HERDR_ENV", val)
		if got := InHerdr(); got != want {
			t.Errorf("HERDR_ENV=%q: InHerdr() = %v, want %v", val, got, want)
		}
	}
}

func TestShellLine(t *testing.T) {
	p := HandoffPlan{Bin: "/usr/bin/claude", Args: []string{"--session-id", "abc", "Resume the Dossier \"it's\""}}
	want := `exec /usr/bin/claude --session-id abc 'Resume the Dossier "it'\''s"'`
	if got := p.ShellLine(); got != want {
		t.Errorf("ShellLine() = %s, want %s", got, want)
	}
	p = HandoffPlan{Bin: "codex", Args: []string{"hi"}, Env: []string{"DOSSIER_SESSION=s1", "PI_SESSION_ID="}}
	want = `exec env DOSSIER_SESSION=s1 PI_SESSION_ID='' codex hi`
	if got := p.ShellLine(); got != want {
		t.Errorf("ShellLine() = %s, want %s", got, want)
	}
}

func TestLaunchInHerdr(t *testing.T) {
	var calls [][]string
	orig := herdrRun
	defer func() { herdrRun = orig }()
	herdrRun = func(args ...string) ([]byte, error) {
		calls = append(calls, args)
		if args[1] == "split" {
			return []byte(`{"result":{"pane":{"pane_id":"1-3"}}}`), nil
		}
		return []byte(`{}`), nil
	}
	plan := HandoffPlan{Bin: "claude", Args: []string{"x"}, Dir: "/d"}
	if err := LaunchInHerdr(plan); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"pane", "split", "--current", "--direction", "right", "--cwd", "/d", "--focus"},
		{"pane", "run", "1-3", "exec claude x"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("calls = %v, want %v", calls, want)
	}

	herdrRun = func(args ...string) ([]byte, error) { return []byte(`{}`), nil }
	if err := LaunchInHerdr(plan); err == nil {
		t.Error("expected an error when split returns no pane id")
	}
}

func TestLaunchInHerdrLabelsWorkspaceAndTab(t *testing.T) {
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	t.Setenv("HERDR_TAB_ID", "w1:t2")
	var calls [][]string
	orig := herdrRun
	defer func() { herdrRun = orig }()
	herdrRun = func(args ...string) ([]byte, error) {
		calls = append(calls, args)
		if args[1] == "split" {
			return []byte(`{"result":{"pane":{"pane_id":"1-3"}}}`), nil
		}
		return []byte(`{}`), nil
	}
	plan := HandoffPlan{Bin: "claude", Dir: "/d", Slug: "my-dossier"}
	if err := LaunchInHerdr(plan); err != nil {
		t.Fatal(err)
	}
	got := calls[len(calls)-2:]
	want := [][]string{
		{"workspace", "rename", "w1", "my-dossier"},
		{"tab", "rename", "w1:t2", "my-dossier"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rename calls = %v, want %v", got, want)
	}
}
