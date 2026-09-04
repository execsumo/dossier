package core

import (
	"context"
	"testing"
	"time"
)

// TestGetGuideAndGetInstructionsReturnAssetContent proves the ordinary path: the
// Service hands back whatever ReadContextAsset reports, unmodified.
func TestGetGuideAndGetInstructionsReturnAssetContent(t *testing.T) {
	store := newLocalFakeStore()
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: time.Now()}, Config{}, nil)

	if got := svc.GetGuide(); got != "GUIDE BODY" {
		t.Errorf("GetGuide() = %q, want %q", got, "GUIDE BODY")
	}
	if got := svc.GetInstructions(); got != "INSTRUCTIONS BODY" {
		t.Errorf("GetInstructions() = %q, want %q", got, "INSTRUCTIONS BODY")
	}
}

// TestGetGuideAndGetInstructionsReturnEmptyOnlyWhenAssetTrulyMissing guards the
// one legitimate way these methods come back empty: the asset is absent from
// disk *and* the binary (modeled by deleting the key from contextAssets, which
// makes the fake's ReadContextAsset return a not-found error, same as the real
// store when neither the disk projection nor the embed.FS has the name).
func TestGetGuideAndGetInstructionsReturnEmptyOnlyWhenAssetTrulyMissing(t *testing.T) {
	store := newLocalFakeStore()
	delete(store.contextAssets, "guide.md")
	delete(store.contextAssets, "instructions.md")
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: time.Now()}, Config{}, nil)

	if got := svc.GetGuide(); got != "" {
		t.Errorf("GetGuide() = %q, want empty when the asset is missing entirely", got)
	}
	if got := svc.GetInstructions(); got != "" {
		t.Errorf("GetInstructions() = %q, want empty when the asset is missing entirely", got)
	}
}

// TestDoctorAdvisesOnStaleContextAssetsWithoutFailing proves drifted context
// assets are reported as an advisory in Warnings — never as a store-damage Issue,
// since the embedded copy stays authoritative regardless and this is purely a
// "the readable projection is out of date" notice.
func TestDoctorAdvisesOnStaleContextAssetsWithoutFailing(t *testing.T) {
	store := newLocalFakeStore()
	store.staleContextAssets = []string{"guide.md"}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: time.Now()}, Config{}, nil)

	res, err := svc.Doctor(context.Background())
	if err != nil {
		t.Fatalf("Doctor failed: %v", err)
	}
	if !res.OK {
		t.Error("stale context assets must not fail the health check")
	}
	if !warningsContain(res.Warnings, "guide.md") {
		t.Errorf("expected doctor to surface the stale-asset advisory naming guide.md, got %v", res.Warnings)
	}
	report := res.Data.(DoctorReport)
	if len(report.Issues) != 0 {
		t.Errorf("stale context assets must be an advisory, not an Issue; got Issues=%v", report.Issues)
	}
}
