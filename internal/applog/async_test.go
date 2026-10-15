package applog

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestAsyncExportIncludesAcceptedRecords(t *testing.T) {
	store, err := Open(t.TempDir(), DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	a := NewAsyncStore(store)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for i := 0; i < 200; i++ {
		if _, err := a.Write([]byte(fmt.Sprintf("record-%03d\n", i))); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if err := a.Export(ctx, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "record-000") || !strings.Contains(out.String(), "record-199") {
		t.Fatal("export did not flush pending archive records")
	}
	if count := a.Dropped(); count != 0 {
		t.Fatalf("unexpected drops: %d", count)
	}
	if err := a.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestAsyncWriteDoesNotWaitForArchiveDisk(t *testing.T) {
	store, err := Open(t.TempDir(), DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	a := NewAsyncStore(store)
	store.mu.Lock() // simulate slow archive disk path
	done := make(chan struct{})
	go func() { _, _ = a.Write([]byte("a log while archive blocked\n")); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		store.mu.Unlock()
		t.Fatal("log Write blocked waiting for archive mutex")
	}
	store.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestAsyncDropsInsteadOfBlockingOnFullQueue(t *testing.T) {
	store, err := Open(t.TempDir(), DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	a := NewAsyncStore(store)
	store.mu.Lock() // stop archive worker from draining
	for i := 0; i < asyncQueueRecords+50; i++ {
		_, _ = a.Write([]byte("overflow test\n"))
	}
	if count := a.Dropped(); count == 0 {
		store.mu.Unlock()
		t.Fatal("expected to drop non-critical portable archive records")
	}
	store.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
