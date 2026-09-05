package harness

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// PiPointerSchema is the pointer record version the bundled Pi extension writes.
// A pointer from a newer schema is ignored rather than misread.
const PiPointerSchema = 1

// piAncestryDepth bounds the process-ancestry walk. A Dossier process is
// normally a direct child of Pi (MCP server) or a grandchild (bash tool ->
// dossier); the bound keeps a pathological or cyclic /proc read finite.
const piAncestryDepth = 8

// PiSessionPointer is the session identity record the Dossier Pi extension
// publishes for a live Pi process. Pi only exports PI_SESSION_ID/PI_SESSION_FILE
// into the bash tool's spawn environment, so processes Pi starts any other way
// (an MCP server) resolve their session through this file instead. See
// assets/pi-extension.ts and docs/harness-capabilities.md.
type PiSessionPointer struct {
	Schema      int    `json:"schema"`
	PID         int    `json:"pid"`
	SessionID   string `json:"session_id"`
	SessionFile string `json:"session_file,omitempty"`
	CWD         string `json:"cwd,omitempty"`
	Hostname    string `json:"hostname,omitempty"`
	Reason      string `json:"reason,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// PiAgentDir returns Pi's agent/config directory, honouring Pi's own
// PI_CODING_AGENT_DIR override. It does not assert the directory exists.
func PiAgentDir() string {
	if dir := os.Getenv("PI_CODING_AGENT_DIR"); dir != "" {
		return expandTilde(dir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".pi", "agent")
}

// PiSessionPointerDir returns the directory the Pi extension writes session
// pointers to. It lives inside Pi's agent directory so both sides derive it
// from the environment alone; DOSSIER_PI_SESSION_DIR overrides it (the
// extension reads the same variable).
func PiSessionPointerDir() string {
	if dir := os.Getenv("DOSSIER_PI_SESSION_DIR"); dir != "" {
		return expandTilde(dir)
	}
	agentDir := PiAgentDir()
	if agentDir == "" {
		return ""
	}
	return filepath.Join(agentDir, "dossier", "sessions")
}

// LookupPiSessionPointer walks this process's ancestry looking for the session
// pointer published by the Pi process that owns it. Keying on the owning Pi
// process (rather than a single "current session" file) is what keeps
// concurrent Pi sessions from binding each other's Dossiers.
func LookupPiSessionPointer() (*PiSessionPointer, bool) {
	return lookupPiSessionPointer(os.Getpid(), parentPID)
}

func lookupPiSessionPointer(pid int, parentOf func(int) (int, bool)) (*PiSessionPointer, bool) {
	dir := PiSessionPointerDir()
	if dir == "" {
		return nil, false
	}
	for depth := 0; depth < piAncestryDepth && pid > 1; depth++ {
		if p, ok := readPiSessionPointer(dir, pid); ok {
			return p, true
		}
		parent, ok := parentOf(pid)
		if !ok || parent == pid {
			return nil, false
		}
		pid = parent
	}
	return nil, false
}

func readPiSessionPointer(dir string, pid int) (*PiSessionPointer, bool) {
	data, err := os.ReadFile(filepath.Join(dir, strconv.Itoa(pid)+".json"))
	if err != nil {
		return nil, false
	}
	var p PiSessionPointer
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, false
	}
	if p.SessionID == "" || p.Schema > PiPointerSchema {
		return nil, false
	}
	if p.Hostname != "" {
		host, err := os.Hostname()
		if err == nil && p.Hostname != host {
			return nil, false
		}
	}
	return &p, true
}

// parentPID resolves a process's parent, preferring procfs (Linux) and falling
// back to ps (macOS). A failure is reported as "no parent" so callers stop
// walking rather than guess.
func parentPID(pid int) (int, bool) {
	if data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if !strings.HasPrefix(line, "PPid:") {
				continue
			}
			ppid, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "PPid:")))
			if err != nil {
				return 0, false
			}
			return ppid, true
		}
		return 0, false
	}

	out, err := exec.Command("ps", "-o", "ppid=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, false
	}
	ppid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, false
	}
	return ppid, true
}

func expandTilde(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		if path == "~" {
			return home
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
