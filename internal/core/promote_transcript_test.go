package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type promoteTranscriptStore struct {
	*localFakeStore
	artifactCalls int
	auditCalls    int
	failArtifact  int
	failAudit     int
}

func (s *promoteTranscriptStore) WriteArtifact(id string, art *Artifact) error {
	s.artifactCalls++
	if s.artifactCalls == s.failArtifact {
		return errors.New("injected artifact write failure")
	}
	return s.localFakeStore.WriteArtifact(id, art)
}

func (s *promoteTranscriptStore) AppendAudit(id string, event AuditEvent) error {
	s.auditCalls++
	if s.auditCalls == s.failAudit {
		return errors.New("injected audit write failure")
	}
	return s.localFakeStore.AppendAudit(id, event)
}

func newPromoteTranscriptService(store Store) *Service {
	return NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{},
		&mockClock{now: time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)}, Config{}, nil)
}

func TestPromoteArchivesRawJSONLBeforeCompiledView(t *testing.T) {
	store := &promoteTranscriptStore{localFakeStore: newLocalFakeStore()}
	svc := newPromoteTranscriptService(store)

	res, err := svc.Promote(context.Background(), PromoteReq{
		Name: "Billing transcript", Content: sampleTrace, Force: true,
	})
	if err != nil {
		t.Fatalf("Promote() error = %v", err)
	}
	id := res.Data.(string)
	artifacts := store.artifacts[id]
	if len(artifacts) != 2 {
		t.Fatalf("Promote() archived %d transcript artifacts, want raw + compiled", len(artifacts))
	}

	raw, compiled := artifacts[0], artifacts[1]
	if raw.Content != sampleTrace {
		t.Fatalf("raw artifact bytes changed:\n got %q\nwant %q", raw.Content, sampleTrace)
	}
	if raw.ContentFormat != ContentFormatText || !strings.Contains(raw.Title, "Raw") ||
		!strings.Contains(raw.Provenance.Origin, "byte-preserved") {
		t.Errorf("raw artifact metadata is not explicit: %+v", raw)
	}
	if compiled.Content == sampleTrace || compiled.ContentFormat != ContentFormatMarkdown ||
		!strings.Contains(compiled.Content, "## [2] tool_call Bash") {
		t.Errorf("compiled artifact is not the citable view: %+v", compiled)
	}
	if !strings.Contains(compiled.Provenance.Origin, raw.ID) {
		t.Errorf("compiled provenance %q does not identify raw artifact %s", compiled.Provenance.Origin, raw.ID)
	}

	var rawAudited, compiledAudited bool
	for _, event := range store.audits[id] {
		if len(event.ArtifactsAdded) != 1 {
			continue
		}
		switch event.ArtifactsAdded[0] {
		case raw.ID:
			rawAudited = strings.Contains(event.Message, "byte-preserved raw")
		case compiled.ID:
			compiledAudited = strings.Contains(event.Message, raw.ID)
		}
	}
	if !rawAudited || !compiledAudited {
		t.Errorf("transcript provenance was not clearly audited: %+v", store.audits[id])
	}
}

func TestPromotePlainTextTranscriptRemainsOneArtifact(t *testing.T) {
	store := &promoteTranscriptStore{localFakeStore: newLocalFakeStore()}
	svc := newPromoteTranscriptService(store)
	raw := "A plain text transcript.\nSecond line."

	res, err := svc.Promote(context.Background(), PromoteReq{
		Name: "Plain transcript", Content: raw, Force: true,
	})
	if err != nil {
		t.Fatalf("Promote() error = %v", err)
	}
	artifacts := store.artifacts[res.Data.(string)]
	if len(artifacts) != 1 {
		t.Fatalf("Promote() archived %d artifacts, want one passthrough artifact", len(artifacts))
	}
	if artifacts[0].Content != raw || artifacts[0].ContentFormat != ContentFormatText {
		t.Errorf("plain-text artifact was not passed through verbatim: %+v", artifacts[0])
	}
	if !strings.Contains(artifacts[0].Provenance.Origin, "verbatim plain-text") {
		t.Errorf("plain-text provenance is unclear: %q", artifacts[0].Provenance.Origin)
	}
}

func TestPromotePreservesRawWhenCompiledArtifactWriteFails(t *testing.T) {
	store := &promoteTranscriptStore{localFakeStore: newLocalFakeStore(), failArtifact: 2}
	svc := newPromoteTranscriptService(store)

	_, err := svc.Promote(context.Background(), PromoteReq{
		Name: "Write failure", Content: sampleTrace, Force: true,
	})
	if err == nil || !strings.Contains(err.Error(), "injected artifact write failure") {
		t.Fatalf("Promote() error = %v, want compiled artifact write failure", err)
	}
	artifacts := store.artifacts["dos_fake_id"]
	if len(artifacts) != 1 || artifacts[0].Content != sampleTrace || !strings.Contains(artifacts[0].Title, "Raw") {
		t.Fatalf("raw input was not preserved before failure: %+v", artifacts)
	}
}

func TestPromoteReturnsTranscriptAuditWriteErrors(t *testing.T) {
	// Save emits the first audit call; the raw transcript audit is the second.
	store := &promoteTranscriptStore{localFakeStore: newLocalFakeStore(), failAudit: 2}
	svc := newPromoteTranscriptService(store)

	_, err := svc.Promote(context.Background(), PromoteReq{
		Name: "Audit failure", Content: sampleTrace, Force: true,
	})
	if err == nil || !strings.Contains(err.Error(), "injected audit write failure") {
		t.Fatalf("Promote() error = %v, want transcript audit failure", err)
	}
	artifacts := store.artifacts["dos_fake_id"]
	if len(artifacts) != 1 || artifacts[0].Content != sampleTrace {
		t.Fatalf("raw input should remain archived when its audit fails: %+v", artifacts)
	}
}
