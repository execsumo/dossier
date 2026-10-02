package mcp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"dossier/internal/core"
	"dossier/internal/store"
)

// failedSyncer reports a persisted failed last sync via the local status.
type failedSyncer struct{ blockingSyncer }

func (f *failedSyncer) LocalStatus(ctx context.Context) (core.SyncStatus, error) {
	return core.SyncStatus{LastError: "dial timeout", AuthState: "ok"}, nil
}

func TestDossierListCarriesSyncWarning(t *testing.T) {
	svc := core.NewService(store.NewFakeStore(), &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, core.Config{}, &failedSyncer{})
	out := &safeWriter{}
	server := NewServer(svc, bytes.NewBuffer(nil), out)
	server.handleRequest(context.Background(), JSONRPCRequest{JSONRPC: "2.0", Method: "tools/call", Params: []byte(`{"name":"dossier_list","arguments":{}}`), ID: 1})
	if got := out.String(); !strings.Contains(got, "last sync failed") {
		t.Fatalf("dossier_list must surface a failed sync, got: %s", got)
	}
}
