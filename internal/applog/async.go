package applog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	asyncQueueRecords = 1024
	asyncQueueBytes   = 8 << 20 // 8 MiB across all queued records
)

type archiveJob struct {
	record  []byte
	barrier chan struct{}
}

// AsyncStore duplicates application logs to a best-effort portable archive.
// It never waits for archive disk I/O on the logging caller's goroutine.
// The primary OS log destination must remain a separate synchronous writer.
type AsyncStore struct {
	store *Store
	queue chan archiveJob
	done  chan struct{}

	mu          sync.Mutex // protects closed, queuedBytes, and queue sends/close
	closed      bool
	queuedBytes int
	dropped     atomic.Uint64
}

func NewAsyncStore(store *Store) *AsyncStore {
	a := &AsyncStore{
		store: store,
		queue: make(chan archiveJob, asyncQueueRecords),
		done:  make(chan struct{}),
	}
	go a.run()
	return a
}

func (a *AsyncStore) Write(p []byte) (int, error) {
	if a == nil {
		return len(p), nil
	}
	// The logger may reuse p as soon as this method returns.
	data := truncateRecord(p, a.store.opts.MaxRecordBytes)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.queuedBytes+len(data) > asyncQueueBytes {
		if !a.closed {
			a.dropped.Add(1)
		}
		return len(p), nil
	}
	// No disk I/O or waiting for queue space. Check record capacity before
	// allocating: producers are serialized by mu, and consumer only removes.
	if len(a.queue) == cap(a.queue) {
		a.dropped.Add(1)
		return len(p), nil
	}
	a.queue <- archiveJob{record: append([]byte(nil), data...)}
	a.queuedBytes += len(data)
	return len(p), nil
}

func (a *AsyncStore) run() {
	defer close(a.done)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	var lastReported uint64
	for {
		select {
		case job, ok := <-a.queue:
			if !ok {
				return
			}
			if job.barrier != nil {
				close(job.barrier)
				continue
			}
			_, _ = a.store.Write(job.record)
			a.mu.Lock()
			a.queuedBytes -= len(job.record)
			a.mu.Unlock()
		case <-ticker.C:
			total := a.dropped.Load()
			if total > lastReported {
				// Avoid log.Printf: routing back through this queue is recursion.
				fmt.Fprintf(os.Stderr, "WARNING: portable archive dropped %d log records in the last minute (total=%d); primary OS logging unaffected\n", total-lastReported, total)
				lastReported = total
			}
		}
	}
}

// Flush is a FIFO barrier. It includes every record accepted before the
// barrier, but does not perform fsync (the Store's Export does that itself).
func (a *AsyncStore) Flush(ctx context.Context) error {
	if a == nil {
		return errors.New("application log archive unavailable")
	}
	barrier := make(chan struct{})
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		a.mu.Lock()
		if a.closed {
			a.mu.Unlock()
			return errors.New("application log archive closing")
		}
		queued := false
		select {
		case a.queue <- archiveJob{barrier: barrier}:
			queued = true
		default:
		}
		a.mu.Unlock()
		if queued {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	select {
	case <-barrier:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *AsyncStore) Export(ctx context.Context, dst io.Writer) error {
	if err := a.Flush(ctx); err != nil {
		return err
	}
	return a.store.Export(ctx, dst)
}

func (a *AsyncStore) ExportFilename(now time.Time) string {
	return a.store.ExportFilename(now)
}

func (a *AsyncStore) Dropped() uint64 { return a.dropped.Load() }

// Close accepts a deadline so an unhealthy disk cannot indefinitely delay
// gateway shutdown. On a deadline, do NOT close Store under a live worker.
func (a *AsyncStore) Close(ctx context.Context) error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	if !a.closed {
		a.closed = true
		close(a.queue)
	}
	a.mu.Unlock()
	select {
	case <-a.done:
		if count := a.dropped.Load(); count != 0 {
			fmt.Fprintf(os.Stderr, "WARNING: portable archive dropped %d records total\n", count)
		}
		return a.store.Close()
	case <-ctx.Done():
		return fmt.Errorf("portable log archive drain timed out: %w", ctx.Err())
	}
}
