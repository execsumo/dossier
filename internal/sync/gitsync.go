package sync

import (
	"time"

	"github.com/go-git/go-git/v5/plumbing/transport"
)

const (
	// originName is the conventional remote name used by the spike.
	originName = "origin"
	// MaxFileSizeBytes is GitHub's hard per-file limit; larger files are excluded
	// from sync commits and reported, never committed (degrade visibly).
	MaxFileSizeBytes int64 = 100 * 1024 * 1024
)

// Config configures a [GitSync] adapter.
type Config struct {
	// AuthorName / AuthorEmail stamp synthesized git commits.
	AuthorName  string
	AuthorEmail string
	// RemoteURL is the git URL (or local path for the spike) of the team bare repo.
	RemoteURL string
	// StoreDir is the working tree (= DOSSIER_HOME).
	StoreDir string
	// Branch is the tracked branch name; defaults to "main".
	Branch string
	// LockTimeout bounds how long a second [GitSync.Sync] waits for the store
	// lock. Defaults to 30s.
	LockTimeout time.Duration
	// Auth is the optional transport credentials.
	Auth transport.AuthMethod
}

// GitSync is the Phase-2 spike git-sync adapter. It is NOT wired into
// core.Service, the CLI, or MCP; see the package godoc and
// docs/spikes/gitsync-findings.md.
type GitSync struct {
	cfg Config
}

// New returns a GitSync with defaults applied.
func New(cfg Config) *GitSync {
	if cfg.Branch == "" {
		cfg.Branch = "main"
	}
	if cfg.LockTimeout == 0 {
		cfg.LockTimeout = defaultLockTimeout()
	}
	return &GitSync{cfg: cfg}
}

// SyncReport is the result of one [GitSync.Sync] run.
type SyncReport struct {
	// Pulled is true if remote changes were merged into the working tree.
	Pulled bool
	// Pushed is true if local commits were pushed to the remote.
	Pushed bool
	// CommitSHA is the SHA of the local commit created this sync ("" if none).
	CommitSHA string
	// CommitMessage is the synthesized commit message.
	CommitMessage string
	// Conflicts lists both-modified files: remote won the working tree, the local
	// version is captured for the caller to route into conflict machinery.
	Conflicts []ConflictRecord
	// Excluded lists oversized files refused from the commit.
	Excluded []ExcludedFile
	// Ahead / Behind counts versus the remote after sync.
	Ahead  int
	Behind int
	// Error carries a human-readable sync-phase error (fetch/push failure) when
	// the local commit still landed but the remote could not be reached. "" on
	// success.
	Error string
}

// ConflictRecord captures a both-modified file: remote content wins the working
// tree; local content is preserved for the caller (the future Phase-2 wiring
// routes this into core's conflicts/*.md machinery). No git merge markers exist
// anywhere in the store.
type ConflictRecord struct {
	// Path is the repo-relative path both sides modified (e.g. "pricing/dossier.md").
	Path string
	// LocalContent is the local version's full content (captured before remote wins).
	LocalContent []byte
	// RemoteContent is the remote version's full content (landed in the working tree).
	RemoteContent []byte
	// LocalRevision / RemoteRevision are the commit SHAs that introduced each side.
	LocalRevision  string
	RemoteRevision string
}

// ExcludedFile describes an oversized file refused from the commit.
type ExcludedFile struct {
	Path    string
	Size    int64
	Warning string
}

// SyncStatus is a read-only snapshot (no push/pull mutation).
type SyncStatus struct {
	Ahead     int
	Behind    int
	LastSync  time.Time
	Conflicts []ConflictRecord // pending conflicts from the last sync
	Dirty     int              // uncommitted tracked changes
}
