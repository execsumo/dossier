package mcp

import (
	"bytes"
	"context"
	"dossier/internal/core"
	"dossier/internal/store"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type blockingSyncer struct {
	syncCalls int32
	blockChan chan struct{}
	entered   chan struct{}
	doneChan  chan struct{}
}

func (b *blockingSyncer) Sync(ctx context.Context) (core.SyncReport, error) {
	atomic.AddInt32(&b.syncCalls, 1)
	if b.entered != nil {
		b.entered <- struct{}{}
	}
	if b.blockChan != nil {
		<-b.blockChan
	}
	if b.doneChan != nil {
		b.doneChan <- struct{}{}
	}
	return core.SyncReport{}, nil
}
func (b *blockingSyncer) Status(ctx context.Context) (core.SyncStatus, error) {
	return core.SyncStatus{}, nil
}
func (b *blockingSyncer) Create(ctx context.Context) error                            { return nil }
func (b *blockingSyncer) Clone(ctx context.Context, url, dir string, depth int) error { return nil }

func TestServiceConcurrentVocabularyMutationAndDebouncedSync(t *testing.T) {
	fakeStore := store.NewFakeStore()
	syncer := &blockingSyncer{
		blockChan: make(chan struct{}),
		entered:   make(chan struct{}, 1),
	}
	svc := core.NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{},
		&mockClock{}, core.Config{Interfaces: []string{"Planning"}}, syncer)
	server := NewServer(svc, bytes.NewBuffer(nil), &safeWriter{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.syncDebouncer(ctx)
	server.triggerSync()
	close(server.syncChan)

	select {
	case <-syncer.entered:
	case <-time.After(time.Second):
		t.Fatal("debounced Sync did not start")
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			svc.AddLead("Alice")
			svc.AddInterface("Review")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = svc.Leads()
			_ = svc.Interfaces()
			if _, err := svc.Save(context.Background(), core.SaveReq{
				FrontmatterUpdates: map[string]any{"name": "Concurrent config read"},
			}); err != nil {
				t.Errorf("Save() error = %v", err)
				return
			}
		}
	}()
	wg.Wait()
	close(syncer.blockChan)

	select {
	case <-server.doneChan:
	case <-time.After(time.Second):
		t.Fatal("debouncer did not stop")
	}
}

func TestMCPServer_Debouncer_NonBlockingAndDrain(t *testing.T) {
	fakeStore := store.NewFakeStore()
	hreg := &mockHarnessRegistry{}
	clk := &mockClock{}
	tok := &mockTokenizer{}
	srch := &mockSearcher{}
	cfg := core.Config{}

	d := &core.Dossier{
		Frontmatter:    core.Frontmatter{ID: "dos_1", Name: "Test Dossier"},
		DistilledState: core.DistilledState{Body: "old"},
	}
	fakeStore.Dossiers["dos_1"] = d
	fakeStore.Revisions["dos_1"] = "rev_1"

	syncer := &blockingSyncer{
		blockChan: make(chan struct{}),
		doneChan:  make(chan struct{}, 1),
	}
	svc := core.NewService(fakeStore, srch, tok, hreg, clk, cfg, syncer)

	reqStr := `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"dossier_save","arguments":{"id":"dos_1","distilled_state_markdown":"# New"}},"id":1}`
	inBuf := bytes.NewBufferString(reqStr + "\n")
	outBuf := &safeWriter{}
	server := NewServer(svc, inBuf, outBuf)

	errCh := make(chan error)
	go func() {
		errCh <- server.Run(context.Background())
	}()

	// Wait for response to be written (non-blocking)
	time.Sleep(200 * time.Millisecond)
	if !strings.Contains(outBuf.String(), `"id":1`) {
		t.Fatalf("Expected tool call to return immediately, but it did not. Output: %s", outBuf.String())
	}

	// Unblock the sync
	close(syncer.blockChan)

	// EOF already sent (inBuf EOF). Wait for server to exit.
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Server run error: %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatalf("Server drain timed out")
	}

	if atomic.LoadInt32(&syncer.syncCalls) != 1 {
		t.Fatalf("Expected exactly 1 sync call, got %d", syncer.syncCalls)
	}
}

func TestMCPServer_Debouncer_Coalesce(t *testing.T) {
	fakeStore := store.NewFakeStore()
	hreg := &mockHarnessRegistry{}
	clk := &mockClock{}
	tok := &mockTokenizer{}
	srch := &mockSearcher{}
	cfg := core.Config{}

	syncer := &blockingSyncer{}
	svc := core.NewService(fakeStore, srch, tok, hreg, clk, cfg, syncer)

	inBuf := bytes.NewBuffer(nil)
	var outBuf bytes.Buffer
	server := NewServer(svc, inBuf, &outBuf)

	// Simulate N rapid saves
	for i := 0; i < 5; i++ {
		server.triggerSync()
	}

	// Close input to trigger EOF and drain
	errCh := make(chan error)
	go func() {
		errCh <- server.Run(context.Background())
	}()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Server run error: %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatalf("Server drain timed out")
	}

	calls := atomic.LoadInt32(&syncer.syncCalls)
	if calls >= 5 || calls == 0 {
		t.Fatalf("Expected coalescing to <5 and >0 sync calls, got %d", calls)
	}
}

type safeWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeWriter) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeWriter) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}
