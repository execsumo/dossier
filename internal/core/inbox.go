package core

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	InboxPending   = "pending"
	InboxAbsorbed  = "absorbed"
	InboxDismissed = "dismissed"
)

// InboxSource describes the external origin without retaining credentials.
type InboxSource struct {
	Kind string `json:"kind" yaml:"kind"`
	URL  string `json:"url,omitempty" yaml:"url,omitempty"`
}

// InboxItem is routed, unverified intake kept separately from Archive evidence.
type InboxItem struct {
	ID         string      `json:"id" yaml:"id"`
	DossierID  string      `json:"dossier_id" yaml:"dossier_id"`
	Source     InboxSource `json:"source" yaml:"source"`
	Excerpt    string      `json:"excerpt" yaml:"excerpt"`
	RoutedBy   string      `json:"routed_by" yaml:"routed_by"`
	Confidence float64     `json:"confidence" yaml:"confidence"`
	ReceivedAt time.Time   `json:"received_at" yaml:"received_at"`
	State      string      `json:"state" yaml:"state"`
	ArtifactID string      `json:"artifact_id,omitempty" yaml:"artifact_id,omitempty"`
}

// InboxStore is an optional machine-local store capability for routed intake.
type InboxStore interface {
	CreateInbox(item *InboxItem) error
	ListInbox(dossierID string) ([]InboxItem, error)
	ReadInbox(dossierID, inboxID string) (*InboxItem, error)
	WriteInbox(item *InboxItem) error
	ValidateInbox(dossierID string) []string
}

type InboxCreateReq struct {
	ID         string
	Source     InboxSource
	Excerpt    string
	RoutedBy   string
	Confidence float64
}
type InboxListReq struct{ ID string }
type InboxReadReq struct {
	ID      string
	InboxID string
}
type InboxResolveReq struct {
	ID      string
	InboxID string
	Action  string // absorb | dismiss
	Actor   string
}

func (s *Service) CreateInbox(ctx context.Context, req InboxCreateReq) (Result, error) {
	store, ok := s.store.(InboxStore)
	if !ok {
		return Result{OK: false}, NewError(ErrInternal, "inbox storage is not configured")
	}
	dossier, _, err := s.store.Read(req.ID)
	if err != nil {
		return Result{OK: false}, err
	}
	if strings.TrimSpace(req.Source.Kind) == "" || strings.TrimSpace(req.Excerpt) == "" || req.Confidence < 0 || req.Confidence > 1 {
		return Result{OK: false}, NewError(ErrInvalidFrontmatter, "inbox source kind and excerpt are required; confidence must be between 0 and 1")
	}
	actor := strings.TrimSpace(req.RoutedBy)
	if actor == "" {
		actor = s.cfg.Author
	}
	item := &InboxItem{DossierID: dossier.Frontmatter.ID, Source: req.Source, Excerpt: req.Excerpt, RoutedBy: actor, Confidence: req.Confidence, ReceivedAt: s.clock.Now(), State: InboxPending}
	if err := store.CreateInbox(item); err != nil {
		return Result{OK: false}, err
	}
	return Result{OK: true, Data: item}, nil
}

func (s *Service) Inbox(ctx context.Context, req InboxListReq) (Result, error) {
	store, ok := s.store.(InboxStore)
	if !ok {
		return Result{OK: false}, NewError(ErrInternal, "inbox storage is not configured")
	}
	dossier, _, err := s.store.Read(req.ID)
	if err != nil {
		return Result{OK: false}, err
	}
	items, err := store.ListInbox(dossier.Frontmatter.ID)
	if err != nil {
		return Result{OK: false}, err
	}
	return Result{OK: true, Data: items}, nil
}

func (s *Service) ReadInbox(ctx context.Context, req InboxReadReq) (Result, error) {
	store, ok := s.store.(InboxStore)
	if !ok {
		return Result{OK: false}, NewError(ErrInternal, "inbox storage is not configured")
	}
	dossier, _, err := s.store.Read(req.ID)
	if err != nil {
		return Result{OK: false}, err
	}
	item, err := store.ReadInbox(dossier.Frontmatter.ID, req.InboxID)
	if err != nil {
		return Result{OK: false}, err
	}
	return Result{OK: true, Data: item}, nil
}

func (s *Service) ResolveInbox(ctx context.Context, req InboxResolveReq) (Result, error) {
	store, ok := s.store.(InboxStore)
	if !ok {
		return Result{OK: false}, NewError(ErrInternal, "inbox storage is not configured")
	}
	dossier, revision, err := s.store.Read(req.ID)
	if err != nil {
		return Result{OK: false}, err
	}
	item, err := store.ReadInbox(dossier.Frontmatter.ID, req.InboxID)
	if err != nil {
		return Result{OK: false}, err
	}
	if item.State != InboxPending {
		return Result{OK: false}, NewError(ErrConflictDetected, fmt.Sprintf("inbox item %q is already %s", item.ID, item.State))
	}
	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "dismiss":
		item.State = InboxDismissed
	case "absorb":
		actor := strings.TrimSpace(req.Actor)
		if actor == "" {
			actor = s.cfg.Author
		}
		title := fmt.Sprintf("Routed inbox item %s", item.ID)
		artifact := Artifact{Type: ArtifactTypeLink, Title: title, ContentFormat: ContentFormatMarkdown, Content: item.Excerpt, CapturedAt: item.ReceivedAt, RefreshedAt: item.ReceivedAt, Provenance: Provenance{Origin: item.Source.Kind, URL: item.Source.URL, CapturedBy: actor}}
		saved, err := s.Save(ctx, SaveReq{ID: dossier.Frontmatter.ID, BaseRevision: revision, Actor: req.Actor, DistilledStateMarkdown: dossier.DistilledState.Body, Artifacts: []Artifact{artifact}})
		if err != nil {
			return Result{OK: false}, err
		}
		artifacts, err := s.store.ListArtifacts(dossier.Frontmatter.ID)
		if err != nil {
			return Result{OK: false}, err
		}
		for _, archived := range artifacts {
			if archived.Title == title {
				item.ArtifactID = archived.ID
				break
			}
		}
		if item.ArtifactID == "" {
			return Result{OK: false}, NewError(ErrInternal, "absorbed inbox artifact was not present in the archive index")
		}
		item.State = InboxAbsorbed
		if err := store.WriteInbox(item); err != nil {
			return Result{OK: false}, err
		}
		actions := []NextAction{"Cite the new Archive artifact in the Distilled State when its content supports a material claim."}
		return Result{OK: true, Data: map[string]any{"item": item, "artifact_id": item.ArtifactID, "revision": saved.Data}, NextActions: actions, Warnings: saved.Warnings}, nil
	default:
		return Result{OK: false}, NewError(ErrInvalidFrontmatter, "inbox action must be absorb or dismiss")
	}
	if err := store.WriteInbox(item); err != nil {
		return Result{OK: false}, err
	}
	return Result{OK: true, Data: map[string]any{"item": item}}, nil
}
