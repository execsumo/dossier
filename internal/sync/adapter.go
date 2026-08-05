package sync

import (
	"context"
	"dossier/internal/core"
)

// Adapter wraps GitSync and maps its internal types to core.Syncer types.
type Adapter struct {
	gs *GitSync
}

// NewAdapter creates a new Adapter.
func NewAdapter(gs *GitSync) *Adapter {
	return &Adapter{gs: gs}
}

// Sync runs the full sync cycle and maps the report to the core DTO.
func (a *Adapter) Sync(ctx context.Context) (core.SyncReport, error) {
	report, err := a.gs.syncWithCtx(ctx)
	if err != nil {
		return core.SyncReport{}, err
	}

	var conflicts []core.SyncConflict
	for _, c := range report.Conflicts {
		conflicts = append(conflicts, core.SyncConflict{
			Path:           c.Path,
			LocalContent:   c.LocalContent,
			RemoteContent:  c.RemoteContent,
			LocalRevision:  c.LocalRevision,
			RemoteRevision: c.RemoteRevision,
		})
	}

	var excluded []core.SyncExcluded
	for _, e := range report.Excluded {
		excluded = append(excluded, core.SyncExcluded{
			Path:    e.Path,
			Size:    e.Size,
			Warning: e.Warning,
		})
	}

	return core.SyncReport{
		Pulled:        report.Pulled,
		Pushed:        report.Pushed,
		CommitSHA:     report.CommitSHA,
		CommitMessage: report.CommitMessage,
		Conflicts:     conflicts,
		Excluded:      excluded,
		Ahead:         report.Ahead,
		Behind:        report.Behind,
		Error:         report.Error,
	}, nil
}

// Status returns a read-only snapshot mapped to the core DTO.
func (a *Adapter) Status(ctx context.Context) (core.SyncStatus, error) {
	st, err := a.gs.Status()
	if err != nil {
		return core.SyncStatus{}, err
	}

	var conflicts []core.SyncConflict
	for _, c := range st.Conflicts {
		conflicts = append(conflicts, core.SyncConflict{
			Path:           c.Path,
			LocalContent:   c.LocalContent,
			RemoteContent:  c.RemoteContent,
			LocalRevision:  c.LocalRevision,
			RemoteRevision: c.RemoteRevision,
		})
	}

	return core.SyncStatus{
		Ahead:     st.Ahead,
		Behind:    st.Behind,
		LastSync:  st.LastSync,
		Conflicts: conflicts,
		Dirty:     st.Dirty,
	}, nil
}

// Create implements core.Syncer.
func (a *Adapter) Create(ctx context.Context) error {
	return a.gs.Create(ctx)
}

// Clone implements core.Syncer.
func (a *Adapter) Clone(ctx context.Context, url, dir string, depth int) error {
	return a.gs.Clone(url, dir, depth)
}
