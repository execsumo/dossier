package core

import (
	"context"
	"time"
)

// Revision represents a content revision hash.
type Revision string

// Store defines the CRUD contract for persistence.
type Store interface {
	Init() error
	Read(slugOrID string) (*Dossier, Revision, error)
	ReadRevision(slugOrID string, rev Revision) (*Dossier, error)
	List(statusFilter string) ([]Frontmatter, error)
	Write(d *Dossier, base Revision) (Revision, error)
	WriteArtifact(dossierID string, a *Artifact) error
	ReadArtifact(dossierID string, artifactID string) (*Artifact, error)
	ListArtifacts(dossierID string) ([]Artifact, error)
	AppendAudit(dossierID string, e AuditEvent) error
	ReadAuditLog(dossierID string) ([]AuditEvent, error)
	ValidateAuditShards(dossierID string) []string
	EnsureAuditDir(dossierID string) error
	WriteSessionStash(dossierID string, author string, sessionID string, content string) error

	// Session bindings
	SaveSessionBinding(binding *SessionBinding) error
	GetSessionBinding(sessionID string) (*SessionBinding, error)
	ClearSessionBinding(sessionID string) error

	// Conflicts
	WriteConflict(conflict *Conflict) error
	ReadConflict(conflictID string) (*Conflict, error)
	ListConflicts() ([]Conflict, error)

	// Context library
	WriteLibraryContext(data LibraryData) error
}

// LibraryDossier represents a dossier summarized in the context library.
type LibraryDossier struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	Slug          string `json:"slug"`
	NextAction    string `json:"next_action"`
	PriorityScore int    `json:"priority_score"`
}

// LibraryData is the input data used to render the context library.md template.
type LibraryData struct {
	Harness      string           `json:"harness"`
	Capabilities map[string]bool  `json:"capabilities"`
	Warnings     []string         `json:"warnings"`
	OpenDossiers []LibraryDossier `json:"open_dossiers"`
}

// SearchScope limits the scope of search operations.
type SearchScope struct {
	DossierID string // Empty means global search
}

// Hit represents a single search result.
type Hit struct {
	DossierID   string `json:"dossier_id"`
	DossierName string `json:"dossier_name"`
	ArtifactID  string `json:"artifact_id,omitempty"`
	Title       string `json:"title"`
	Path        string `json:"path"`
	Snippet     string `json:"snippet"`
	LineNumber  int    `json:"line_number,omitempty"`
}

// Searcher defines the interface for text searches.
type Searcher interface {
	Search(ctx context.Context, query string, scope SearchScope) ([]Hit, error)
}

// Tokenizer defines the interface for token estimation.
type Tokenizer interface {
	Estimate(text string) int
}

// InstallOpts controls hook installation options.
type InstallOpts struct {
	Interactive      bool
	YesToAll         bool
	StableBinaryPath string
}

// Harness defines the capability detection and integration installer for a client.
type Harness interface {
	Name() string
	Detect() (Capabilities, error)
	Install(opts InstallOpts) error
}

// HarnessRegistry manages the set of supported client harnesses.
type HarnessRegistry interface {
	All() []Harness
	Get(name string) (Harness, error)
}

// Clock defines a mockable interface for wall time.
type Clock interface {
	Now() time.Time
}

// SyncConflict captures a both-modified file.
type SyncConflict struct {
	Path           string
	LocalContent   []byte
	RemoteContent  []byte
	LocalRevision  string
	RemoteRevision string
}

// SyncExcluded describes an oversized file refused from the commit.
type SyncExcluded struct {
	Path    string
	Size    int64
	Warning string
}

// SyncReport is the result of one Sync run.
type SyncReport struct {
	Pulled        bool
	Pushed        bool
	CommitSHA     string
	CommitMessage string
	Conflicts     []SyncConflict
	Excluded      []SyncExcluded
	Ahead         int
	Behind        int
	Error         string
}

// SyncStatus is a read-only snapshot.
type SyncStatus struct {
	Ahead     int
	Behind    int
	LastSync  time.Time
	Conflicts []SyncConflict
	Dirty     int
}

// Syncer defines the port for synchronizing the dossier store with a remote.
type Syncer interface {
	Sync(ctx context.Context) (SyncReport, error)
	Status(ctx context.Context) (SyncStatus, error)
	Create(ctx context.Context) error
	Clone(ctx context.Context, url, dir string, depth int) error
}
