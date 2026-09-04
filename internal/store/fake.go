package store

import (
	"dossier/assets"
	"dossier/internal/core"
	"fmt"
	"time"
)

// FakeStore implements core.Store in-memory for core unit tests.
type FakeStore struct {
	Dossiers  map[string]*core.Dossier
	Revisions map[string]core.Revision
	Artifacts map[string][]core.Artifact
	Audits    map[string][]core.AuditEvent
	Sessions  map[string]*core.SessionBinding
	Conflicts map[string]*core.Conflict
	History   map[core.Revision]*core.Dossier
}

// NewFakeStore instantiates an in-memory FakeStore.
func NewFakeStore() *FakeStore {
	return &FakeStore{
		Dossiers:  make(map[string]*core.Dossier),
		Revisions: make(map[string]core.Revision),
		Artifacts: make(map[string][]core.Artifact),
		Audits:    make(map[string][]core.AuditEvent),
		Sessions:  make(map[string]*core.SessionBinding),
		Conflicts: make(map[string]*core.Conflict),
		History:   make(map[core.Revision]*core.Dossier),
	}
}

func (f *FakeStore) Init() error {
	return nil
}

func cloneDossier(d *core.Dossier) *core.Dossier {
	cp := *d
	cp.Frontmatter.Interfaces = append([]string(nil), d.Frontmatter.Interfaces...)
	return &cp
}

func fakeSlugMatches(d *core.Dossier, value string) bool {
	if d.Frontmatter.ID == value || d.Frontmatter.Slug == value {
		return true
	}
	return false
}

func (f *FakeStore) Read(slugOrID string) (*core.Dossier, core.Revision, error) {
	d, ok := f.Dossiers[slugOrID]
	if !ok {
		// Try by ID or Slug check
		for _, dos := range f.Dossiers {
			if fakeSlugMatches(dos, slugOrID) {
				return cloneDossier(dos), f.Revisions[dos.Frontmatter.ID], nil
			}
		}
		return nil, "", core.NewError(core.ErrNotFound, fmt.Sprintf("dossier %q not found in fake store", slugOrID))
	}
	return cloneDossier(d), f.Revisions[d.Frontmatter.ID], nil
}

func (f *FakeStore) ReadRevision(slugOrID string, rev core.Revision) (*core.Dossier, error) {
	if d, ok := f.History[rev]; ok {
		return cloneDossier(d), nil
	}
	for _, d := range f.Dossiers {
		if fakeSlugMatches(d, slugOrID) {
			currRev := f.Revisions[d.Frontmatter.ID]
			if currRev == rev {
				return cloneDossier(d), nil
			}
		}
	}
	return nil, core.NewError(core.ErrNotFound, fmt.Sprintf("revision %s not found in fake store", rev))
}

func (f *FakeStore) List(statusFilter string) ([]core.Frontmatter, error) {
	list := []core.Frontmatter{}
	for _, d := range f.Dossiers {
		if statusFilter == "all" || string(d.Frontmatter.Status) == statusFilter {
			list = append(list, d.Frontmatter)
		}
	}
	return list, nil
}

func (f *FakeStore) Write(d *core.Dossier, base core.Revision) (core.Revision, error) {
	id := d.Frontmatter.ID
	if id == "" {
		id = fmt.Sprintf("dos_fake_%d", len(f.Dossiers)+1)
		d.Frontmatter.ID = id
		if d.Frontmatter.Slug == "" {
			d.Frontmatter.Slug = id
		}
	}
	currentRev := f.Revisions[id]
	if currentRev != base {
		return "", core.NewError(core.ErrConcurrentEdit, "concurrent edit detected")
	}

	// Save to history before overwriting
	if currentRev != "" {
		if existing, ok := f.Dossiers[id]; ok {
			cp := *existing
			f.History[currentRev] = &cp
		}
	}

	f.Dossiers[id] = d
	newRev := core.Revision(fmt.Sprintf("rev_fake_%d", len(f.History)+1))
	f.Revisions[id] = newRev
	return newRev, nil
}

func (f *FakeStore) Rename(dossierID string, newSlug string, newName string, base core.Revision) (*core.Dossier, core.Revision, error) {
	if newSlug != "" {
		if err := core.ValidateCanonicalSlug(newSlug); err != nil {
			return nil, "", core.WrapError(core.ErrInvalidFrontmatter, "invalid slug", err)
		}
	}
	d, currentRev, err := f.Read(dossierID)
	if err != nil {
		return nil, "", err
	}
	if base != "" && base != currentRev {
		return nil, "", core.NewError(core.ErrConcurrentEdit, "concurrent edit detected")
	}
	if newSlug == "" {
		newSlug = d.Frontmatter.Slug
	}
	if newName == "" {
		newName = d.Frontmatter.Name
	}
	slugChanged := d.Frontmatter.Slug != newSlug
	nameChanged := d.Frontmatter.Name != newName
	if !slugChanged && !nameChanged {
		return d, currentRev, nil
	}
	if slugChanged {
		for _, other := range f.Dossiers {
			if other.Frontmatter.ID != d.Frontmatter.ID && fakeSlugMatches(other, newSlug) {
				return nil, "", core.NewError(core.ErrInvalidFrontmatter, fmt.Sprintf("slug %q is already used by another dossier", newSlug))
			}
		}
	}
	f.History[currentRev] = cloneDossier(d)
	d.Frontmatter.Slug = newSlug
	d.Frontmatter.Name = newName
	d.Frontmatter.UpdatedAt = time.Now().Truncate(time.Second)
	newRev := core.Revision(fmt.Sprintf("rev_fake_%d", len(f.History)+1))
	f.Dossiers[d.Frontmatter.ID] = cloneDossier(d)
	f.Revisions[d.Frontmatter.ID] = newRev
	return cloneDossier(d), newRev, nil
}

// RenameSlug preserves the original store API for slug-only callers.
func (f *FakeStore) RenameSlug(dossierID string, newSlug string, base core.Revision) (*core.Dossier, core.Revision, error) {
	return f.Rename(dossierID, newSlug, "", base)
}

func (f *FakeStore) WriteArtifact(dossierID string, a *core.Artifact) error {
	if a.ID == "" {
		a.ID = fmt.Sprintf("art_fake_%d", len(f.Artifacts[dossierID])+1)
	}
	a.DossierID = dossierID
	f.Artifacts[dossierID] = append(f.Artifacts[dossierID], *a)
	return nil
}

func (f *FakeStore) ReadArtifact(dossierID string, artifactID string) (*core.Artifact, error) {
	for _, a := range f.Artifacts[dossierID] {
		if a.ID == artifactID {
			return &a, nil
		}
	}
	return nil, core.NewError(core.ErrNotFound, "artifact not found")
}

func (f *FakeStore) ListArtifacts(dossierID string) ([]core.Artifact, error) {
	return f.Artifacts[dossierID], nil
}

func (f *FakeStore) AppendAudit(dossierID string, e core.AuditEvent) error {
	f.Audits[dossierID] = append(f.Audits[dossierID], e)
	return nil
}

func (f *FakeStore) ReadAuditLog(dossierID string) ([]core.AuditEvent, error) {
	return f.Audits[dossierID], nil
}

func (f *FakeStore) ValidateAuditShards(dossierID string) []string {
	return nil
}

func (f *FakeStore) ValidateArtifactFiles(dossierID string) []string {
	return nil
}

func (f *FakeStore) EnsureAuditDir(dossierID string) error {
	return nil
}

func (f *FakeStore) WriteSessionStash(dossierID string, author string, sessionID string, content string) error {
	return nil
}

func (f *FakeStore) SaveSessionBinding(binding *core.SessionBinding) error {
	f.Sessions[binding.SessionBindingID] = binding
	return nil
}

func (f *FakeStore) GetSessionBinding(sessionID string) (*core.SessionBinding, error) {
	binding, ok := f.Sessions[sessionID]
	if !ok {
		return nil, core.NewError(core.ErrNotFound, "session binding not found")
	}
	return binding, nil
}

func (f *FakeStore) ClearSessionBinding(sessionID string) error {
	delete(f.Sessions, sessionID)
	return nil
}

func (f *FakeStore) WriteConflict(conflict *core.Conflict) error {
	f.Conflicts[conflict.ID] = conflict
	return nil
}

func (f *FakeStore) ReadConflict(conflictID string) (*core.Conflict, error) {
	conflict, ok := f.Conflicts[conflictID]
	if !ok {
		return nil, core.NewError(core.ErrNotFound, "conflict not found")
	}
	return conflict, nil
}

func (f *FakeStore) ListConflicts() ([]core.Conflict, error) {
	list := []core.Conflict{}
	for _, c := range f.Conflicts {
		list = append(list, *c)
	}
	return list, nil
}

func (f *FakeStore) WriteLibraryContext(data core.LibraryData) error {
	return nil
}

func (f *FakeStore) EnsureContextAssets() ([]string, error) { return nil, nil }

func (f *FakeStore) ReadContextAsset(name string) (string, error) {
	content, err := assets.FS.ReadFile(name)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func (f *FakeStore) StaleContextAssets() []string { return nil }
