package harness

import (
	"fmt"
	"reflect"
	"sync"
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

func stubHerdr(t *testing.T, fn func(args ...string) ([]byte, error)) {
	t.Helper()
	orig := herdrRun
	t.Cleanup(func() { herdrRun = orig })
	herdrRun = fn
}

func TestLaunchInHerdr(t *testing.T) {
	t.Setenv("HERDR_WORKSPACE_ID", "hub")
	var calls [][]string
	workspaces := `{"result":{"workspaces":[{"workspace_id":"hub","label":"my-dossier"},{"workspace_id":"w5","label":"other"}]}}`
	var listErr error
	stubHerdr(t, func(args ...string) ([]byte, error) {
		calls = append(calls, args)
		switch {
		case args[0] == "workspace" && args[1] == "list":
			return []byte(workspaces), listErr
		case args[0] == "workspace" || args[0] == "tab":
			return []byte(`{"result":{"root_pane":{"pane_id":"w1:p4"},"workspace":{"workspace_id":"w1"},"tab":{"tab_id":"w1:t2"}}}`), nil
		}
		return []byte(`{}`), nil
	})
	plan := HandoffPlan{Bin: "claude", Args: []string{"x"}, Dir: "/d", Slug: "my-dossier"}
	list := []string{"workspace", "list"}
	run := []string{"pane", "run", "w1:p4", "exec claude x"}
	createWS := []string{"workspace", "create", "--cwd", "/d", "--label", "my-dossier", "--focus"}
	tabIn := func(ws string) []string {
		return []string{"tab", "create", "--workspace", ws, "--cwd", "/d", "--label", "my-dossier", "--focus"}
	}
	check := func(name string, workspaceID string, want ...[]string) {
		t.Helper()
		calls = nil
		pane, err := LaunchInHerdr(plan, workspaceID)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if pane != "w1:p4" {
			t.Errorf("%s: pane = %q, want w1:p4", name, pane)
		}
		if !reflect.DeepEqual(calls, want) {
			t.Errorf("%s: calls = %v, want %v", name, calls, want)
		}
	}

	// No workspace carries the slug (the hub's matching label doesn't count):
	// a new one is created and labelled.
	check("no match", "", list, createWS, run)

	// A workspace already carries the slug, e.g. before herdr has detected the
	// agent in it or after a TUI restart: the new session is a tab in it.
	workspaces = `{"result":{"workspaces":[{"workspace_id":"w5","label":"other"},{"workspace_id":"w9","label":"my-dossier"}]}}`
	check("label match", "", list, tabIn("w9"), run)

	// An explicit workspace (its live agent's) skips the lookup.
	check("explicit workspace", "w7", tabIn("w7"), run)

	// The hub is never a target, even if an old-model agent lives there.
	check("hub as target", "hub", list, tabIn("w9"), run)

	// A failed lookup falls back to creating a workspace, not to an error.
	listErr = fmt.Errorf("socket gone")
	check("list failure", "", list, createWS, run)

	stubHerdr(t, func(args ...string) ([]byte, error) { return []byte(`{}`), nil })
	if _, err := LaunchInHerdr(plan, ""); err == nil {
		t.Error("expected an error when create returns no pane id")
	}
}

func TestLaunchInHerdrSerializesWorkspaceReuse(t *testing.T) {
	t.Setenv("HERDR_WORKSPACE_ID", "hub")
	var created bool
	var calls [][]string
	stubHerdr(t, func(args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		switch {
		case reflect.DeepEqual(args, []string{"workspace", "list"}):
			if created {
				return []byte(`{"result":{"workspaces":[{"workspace_id":"w1","label":"same-dossier"}]}}`), nil
			}
			return []byte(`{"result":{"workspaces":[]}}`), nil
		case args[0] == "workspace" && args[1] == "create":
			created = true
			return []byte(`{"result":{"root_pane":{"pane_id":"w1:p1"}}}`), nil
		case args[0] == "tab" && args[1] == "create":
			return []byte(`{"result":{"root_pane":{"pane_id":"w1:p2"}}}`), nil
		default:
			return []byte(`{}`), nil
		}
	})

	plan := HandoffPlan{Bin: "claude", Args: []string{"x"}, Dir: "/d", Slug: "same-dossier"}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := LaunchInHerdr(plan, "")
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Errorf("LaunchInHerdr: %v", err)
		}
	}

	creates, tabs := 0, 0
	for _, args := range calls {
		if len(args) > 1 && args[1] == "create" {
			if args[0] == "workspace" {
				creates++
			} else if args[0] == "tab" {
				tabs++
			}
		}
	}
	if creates != 1 || tabs != 1 {
		t.Errorf("workspace creates=%d tab creates=%d; want one each (calls: %v)", creates, tabs, calls)
	}
}

func TestLabelHerdrHub(t *testing.T) {
	workspace := func(label string, tabs, panes int) []byte {
		return []byte(fmt.Sprintf(`{"result":{"workspace":{"label":%q,"tab_count":%d,"pane_count":%d}}}`, label, tabs, panes))
	}
	var calls [][]string
	var reply []byte
	stubHerdr(t, func(args ...string) ([]byte, error) {
		calls = append(calls, args)
		return reply, nil
	})

	t.Setenv("HERDR_WORKSPACE_ID", "")
	if err := LabelHerdrHub(); err != nil || len(calls) != 0 {
		t.Fatalf("outside a workspace it must be a no-op: err=%v calls=%v", err, calls)
	}

	t.Setenv("HERDR_WORKSPACE_ID", "w88")
	rename := [][]string{{"workspace", "get", "w88"}, {"workspace", "rename", "w88", "dossier"}}
	getOnly := [][]string{{"workspace", "get", "w88"}}
	for _, tc := range []struct {
		name  string
		reply []byte
		want  [][]string
	}{
		{"only the TUI", workspace("g-unit", 1, 1), rename},
		{"TUI is one of several tabs", workspace("g-unit", 2, 1), getOnly},
		{"TUI is split with another pane", workspace("g-unit", 1, 2), getOnly},
		{"already named", workspace("dossier", 1, 1), getOnly},
	} {
		calls, reply = nil, tc.reply
		if err := LabelHerdrHub(); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !reflect.DeepEqual(calls, tc.want) {
			t.Errorf("%s: calls = %v, want %v", tc.name, calls, tc.want)
		}
	}

	calls, reply = nil, []byte(`not json`)
	if err := LabelHerdrHub(); err == nil {
		t.Error("expected an error for unparseable output")
	}
}

const agentListFixture = `{"id":"cli:agent:list","result":{"agents":[
 {"agent":"claude","agent_session":{"kind":"id","value":"c96c9ad3-3904-440a-9e32-019c6a77ae91"},"agent_status":"idle","pane_id":"w2J:p4","state_change_seq":772,"tab_id":"w2J:t2","workspace_id":"w2J"},
 {"agent":"pi","agent_session":{"kind":"path","value":"/home/u/.pi/agent/sessions/--x--/2026-10-06T17-40-54-765Z_01a1124d-db6d-7190-a61c-f0d9f99cf967.jsonl"},"agent_status":"blocked","pane_id":"w2J:p2","state_change_seq":761,"tab_id":"w2J:t1","workspace_id":"w2J"},
 {"agent":"codex","agent_session":{"kind":"id","value":"codex-own-id"},"agent_status":"working","pane_id":"w3:p1","state_change_seq":800,"tab_id":"w3:t1","workspace_id":"w3"},
 {"agent":"hermes","agent_status":"unknown","pane_id":"w4:p1","state_change_seq":5,"tab_id":"w4:t1","workspace_id":"w4"}
]},"type":"agent_list"}`

func TestListHerdrAgentsAndSessionKeys(t *testing.T) {
	stubHerdr(t, func(args ...string) ([]byte, error) {
		if !reflect.DeepEqual(args, []string{"agent", "list"}) {
			t.Fatalf("unexpected call %v", args)
		}
		return []byte(agentListFixture), nil
	})
	agents, err := ListHerdrAgents()
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, a := range agents {
		keys = append(keys, a.SessionKey())
	}
	want := []string{"c96c9ad3-3904-440a-9e32-019c6a77ae91", "01a1124d-db6d-7190-a61c-f0d9f99cf967", "codex-own-id", ""}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("keys = %q, want %q", keys, want)
	}

	stubHerdr(t, func(args ...string) ([]byte, error) { return []byte(`not json`), nil })
	if _, err := ListHerdrAgents(); err == nil {
		t.Error("expected an error for unparseable output")
	}
}

func TestSessionKeyRejectsUnsafeValues(t *testing.T) {
	for _, tc := range []struct{ kind, value string }{
		{"id", "../../etc/passwd"},
		{"id", "a/b"},
		{"path", "/x/2026_../evil.jsonl"},
		{"path", "/x/no-underscore.jsonl"},
		{"other", "abc"},
	} {
		var a HerdrAgent
		a.Session.Kind, a.Session.Value = tc.kind, tc.value
		if got := a.SessionKey(); got != "" {
			t.Errorf("SessionKey(%s %q) = %q, want empty", tc.kind, tc.value, got)
		}
	}
}

func TestMatchHerdrAgents(t *testing.T) {
	stubHerdr(t, func(args ...string) ([]byte, error) { return []byte(agentListFixture), nil })
	agents, _ := ListHerdrAgents()
	bound := map[string]string{
		"c96c9ad3-3904-440a-9e32-019c6a77ae91": "dos_a",
		"01a1124d-db6d-7190-a61c-f0d9f99cf967": "dos_a",
	}
	launched := map[string]string{"w3:p1": "dos_b", "w2J:p4": "dos_b"}
	got := MatchHerdrAgents(agents, bound, launched)
	if len(got) != 2 || len(got["dos_a"]) != 2 || len(got["dos_b"]) != 1 {
		t.Fatalf("matches = %+v", got)
	}
	if got["dos_b"][0].PaneID != "w3:p1" {
		t.Errorf("binding match must win over launched pane: %+v", got["dos_b"])
	}
	if p := MostRecentAgent(got["dos_a"]).PaneID; p != "w2J:p4" {
		t.Errorf("MostRecentAgent = %s, want w2J:p4 (highest state_change_seq)", p)
	}
}

func TestMostUrgentStatus(t *testing.T) {
	mk := func(statuses ...string) []HerdrAgent {
		var out []HerdrAgent
		for _, s := range statuses {
			out = append(out, HerdrAgent{Status: s})
		}
		return out
	}
	for _, tc := range []struct {
		in   []string
		want string
	}{
		{[]string{"idle", "blocked", "working"}, "blocked"},
		{[]string{"idle", "done", "working"}, "done"},
		{[]string{"idle", "working"}, "working"},
		{[]string{"idle", "unknown"}, "idle"},
		{[]string{"unknown"}, "unknown"},
		{[]string{"weird"}, "unknown"},
	} {
		if got := MostUrgentStatus(mk(tc.in...)); got != tc.want {
			t.Errorf("MostUrgentStatus(%v) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestFocusHerdrAgent(t *testing.T) {
	var calls [][]string
	stubHerdr(t, func(args ...string) ([]byte, error) { calls = append(calls, args); return []byte(`{}`), nil })
	if err := FocusHerdrAgent("w2J:p4"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, [][]string{{"agent", "focus", "w2J:p4"}}) {
		t.Errorf("calls = %v", calls)
	}
}
