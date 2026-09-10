package applog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultMaxAge           = 30 * 24 * time.Hour
	defaultPreferredMinAge  = 14 * 24 * time.Hour
	defaultMaxFileBytes     = int64(64 << 20) // 64 MiB
	defaultMaxTotalBytes    = int64(1 << 30)  // 1 GiB
	defaultMaxRecordBytes   = 1 << 20         // 1 MiB
	defaultCleanupEveryByte = int64(8 << 20)  // 8 MiB
)

const truncationMarker = "\n[application log record truncated]\n"

type Options struct {
	MaxAge            time.Duration
	PreferredMinAge   time.Duration
	MaxFileBytes      int64
	MaxTotalBytes     int64
	MaxRecordBytes    int
	CleanupEveryBytes int64
}

func DefaultOptions() Options {
	return Options{
		MaxAge:            defaultMaxAge,
		PreferredMinAge:   defaultPreferredMinAge,
		MaxFileBytes:      defaultMaxFileBytes,
		MaxTotalBytes:     defaultMaxTotalBytes,
		MaxRecordBytes:    defaultMaxRecordBytes,
		CleanupEveryBytes: defaultCleanupEveryByte,
	}
}

type Store struct {
	dir  string
	opts Options

	archiveMu sync.RWMutex
	mu        sync.Mutex

	file          *os.File
	currentPath   string
	currentDay    string
	currentSize   int64
	bytesSinceGC  int64
	fileSequence  uint64
	closed        bool
	writeDisabled bool
}

type logFileSnapshot struct {
	path string
	name string
	size int64
}

type logFileInfo struct {
	path    string
	name    string
	size    int64
	modTime time.Time
}

// Open creates a portable rotating archive rooted at dir.
func Open(dir string, opts Options) (*Store, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("application log directory is required")
	}

	if err := validateOptions(opts); err != nil {
		return nil, err
	}

	absoluteDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve application log directory: %w", err)
	}

	if err := os.MkdirAll(absoluteDir, 0750); err != nil {
		return nil, fmt.Errorf("create application log directory: %w", err)
	}
	if err := os.Chmod(absoluteDir, 0750); err != nil {
		return nil, fmt.Errorf("set application log directory permissions: %w", err)
	}

	store := &Store{
		dir:  absoluteDir,
		opts: opts,
	}

	if err := store.cleanup(); err != nil {
		return nil, fmt.Errorf("prune application logs: %w", err)
	}

	store.archiveMu.RLock()
	store.mu.Lock()
	err = store.rotateLocked(time.Now())
	store.mu.Unlock()
	store.archiveMu.RUnlock()
	if err != nil {
		return nil, fmt.Errorf("create application log file: %w", err)
	}

	return store, nil
}

func validateOptions(opts Options) error {
	switch {
	case opts.MaxAge <= 0:
		return errors.New("application log MaxAge must be greater than zero")
	case opts.PreferredMinAge < 0:
		return errors.New("application log PreferredMinAge cannot be negative")
	case opts.PreferredMinAge > opts.MaxAge:
		return errors.New("application log PreferredMinAge cannot exceed MaxAge")
	case opts.MaxFileBytes <= 0:
		return errors.New("application log MaxFileBytes must be greater than zero")
	case opts.MaxTotalBytes <= 0:
		return errors.New("application log MaxTotalBytes must be greater than zero")
	case opts.MaxFileBytes > opts.MaxTotalBytes:
		return errors.New("application log MaxFileBytes cannot exceed MaxTotalBytes")
	case opts.MaxRecordBytes <= 0:
		return errors.New("application log MaxRecordBytes must be greater than zero")
	case int64(opts.MaxRecordBytes) > opts.MaxFileBytes:
		return errors.New("application log MaxRecordBytes cannot exceed MaxFileBytes")
	case opts.CleanupEveryBytes <= 0:
		return errors.New("application log CleanupEveryBytes must be greater than zero")
	}

	return nil
}

// Write implements io.Writer so Store can be attached to the standard log
// package through io.MultiWriter. If the portable archive can no longer rotate
// or prune safely, it disables only its own writes; the caller's primary OS log
// writer can continue operating.
func (s *Store) Write(p []byte) (int, error) {
	if s == nil {
		return len(p), nil
	}

	originalLen := len(p)
	data := truncateRecord(p, s.opts.MaxRecordBytes)
	now := time.Now()

	s.archiveMu.RLock()
	s.mu.Lock()

	if s.closed || s.writeDisabled {
		s.mu.Unlock()
		s.archiveMu.RUnlock()
		return originalLen, nil
	}

	day := now.UTC().Format("2006-01-02")
	needsRotation := s.file == nil ||
		s.currentDay != day ||
		(s.currentSize > 0 && s.currentSize+int64(len(data)) > s.opts.MaxFileBytes)

	if needsRotation {
		if err := s.rotateLocked(now); err != nil {
			s.disableWritesLocked()
			s.mu.Unlock()
			s.archiveMu.RUnlock()
			reportArchiveFailure("rotate", err)
			return originalLen, nil
		}
	}

	written, err := s.file.Write(data)
	if err != nil || written != len(data) {
		if err == nil {
			err = io.ErrShortWrite
		}
		s.disableWritesLocked()
		s.mu.Unlock()
		s.archiveMu.RUnlock()
		reportArchiveFailure("write", err)
		return originalLen, nil
	}

	s.currentSize += int64(written)
	s.bytesSinceGC += int64(written)
	needsCleanup := s.bytesSinceGC >= s.opts.CleanupEveryBytes
	if needsCleanup {
		s.bytesSinceGC = 0
	}

	s.mu.Unlock()
	s.archiveMu.RUnlock()

	if needsCleanup {
		if err := s.cleanup(); err != nil {
			s.disableWrites(err)
		}
	}

	return originalLen, nil
}

func truncateRecord(p []byte, maxBytes int) []byte {
	if len(p) <= maxBytes {
		return p
	}

	marker := []byte(truncationMarker)
	if maxBytes <= len(marker) {
		return marker[:maxBytes]
	}

	result := make([]byte, 0, maxBytes)
	result = append(result, p[:maxBytes-len(marker)]...)
	result = append(result, marker...)
	return result
}

func reportArchiveFailure(operation string, err error) {
	_, _ = fmt.Fprintf(
		os.Stderr,
		"WARNING: portable application log archive disabled after %s failure: %v\n",
		operation,
		err,
	)
}

func (s *Store) disableWrites(err error) {
	s.archiveMu.Lock()
	s.mu.Lock()
	alreadyDisabled := s.writeDisabled || s.closed
	if !alreadyDisabled {
		s.disableWritesLocked()
	}
	s.mu.Unlock()
	s.archiveMu.Unlock()

	if !alreadyDisabled {
		reportArchiveFailure("maintenance", err)
	}
}

func (s *Store) disableWritesLocked() {
	s.writeDisabled = true
	if s.file != nil {
		_ = s.file.Sync()
		_ = s.file.Close()
		s.file = nil
	}
}

func (s *Store) rotateLocked(now time.Time) error {
	newFile, newPath, err := s.createLogFileLocked(now)
	if err != nil {
		return err
	}

	oldFile := s.file
	s.file = newFile
	s.currentPath = newPath
	s.currentDay = now.UTC().Format("2006-01-02")
	s.currentSize = 0

	if oldFile != nil {
		_ = oldFile.Sync()
		if err := oldFile.Close(); err != nil {
			return fmt.Errorf("close rotated application log: %w", err)
		}
	}

	return nil
}

func (s *Store) createLogFileLocked(now time.Time) (*os.File, string, error) {
	for attempts := 0; attempts < 1000; attempts++ {
		s.fileSequence++
		name := fmt.Sprintf(
			"log-%s-%d-%06d.log",
			now.UTC().Format("20060102T150405.000000000Z"),
			os.Getpid(),
			s.fileSequence,
		)
		path := filepath.Join(s.dir, name)

		file, err := os.OpenFile(
			path,
			os.O_CREATE|os.O_EXCL|os.O_WRONLY,
			0600,
		)
		if err == nil {
			return file, path, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, "", err
		}
	}

	return nil, "", errors.New("could not allocate a unique application log filename")
}

func (s *Store) Export(ctx context.Context, dst io.Writer) error {
	if s == nil {
		return errors.New("application log archive is unavailable")
	}
	if dst == nil {
		return errors.New("application log export destination is required")
	}

	s.archiveMu.RLock()
	defer s.archiveMu.RUnlock()

	snapshots, err := s.snapshots()
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(
		dst,
		"# application log export generated_utc=%s files=%d\n",
		time.Now().UTC().Format(time.RFC3339Nano),
		len(snapshots),
	); err != nil {
		return err
	}

	for _, snapshot := range snapshots {
		if err := ctx.Err(); err != nil {
			return err
		}

		if _, err := fmt.Fprintf(dst, "\n# --- %s ---\n", snapshot.name); err != nil {
			return err
		}

		file, err := os.Open(snapshot.path)
		if err != nil {
			return fmt.Errorf("open application log %q: %w", snapshot.name, err)
		}

		reader := &contextReader{
			ctx:    ctx,
			reader: io.LimitReader(file, snapshot.size),
		}
		_, copyErr := io.Copy(dst, reader)
		closeErr := file.Close()

		if copyErr != nil {
			return fmt.Errorf("export application log %q: %w", snapshot.name, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close application log %q: %w", snapshot.name, closeErr)
		}
	}

	return nil
}

func (s *Store) snapshots() ([]logFileSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.file != nil {
		if err := s.file.Sync(); err != nil {
			return nil, fmt.Errorf("flush active application log: %w", err)
		}
	}

	files, err := s.listLogFiles()
	if err != nil {
		return nil, err
	}

	result := make([]logFileSnapshot, 0, len(files))
	for _, file := range files {
		result = append(result, logFileSnapshot{
			path: file.path,
			name: file.name,
			size: file.size,
		})
	}

	return result, nil
}

// ExportFilename returns a safe attachment name for an exported archive.
func (s *Store) ExportFilename(now time.Time) string {
	return fmt.Sprintf(
		"logs-%s.log",
		now.UTC().Format("20060102T150405Z"),
	)
}

func (s *Store) cleanup() error {
	s.archiveMu.Lock()
	defer s.archiveMu.Unlock()

	s.mu.Lock()
	currentPath := s.currentPath
	s.mu.Unlock()

	files, err := s.listLogFiles()
	if err != nil {
		return err
	}

	now := time.Now()
	maxAgeCutoff := now.Add(-s.opts.MaxAge)

	for _, file := range files {
		if file.path == currentPath || !file.modTime.Before(maxAgeCutoff) {
			continue
		}
		if err := os.Remove(file.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove expired application log %q: %w", file.name, err)
		}
	}

	files, err = s.listLogFiles()
	if err != nil {
		return err
	}

	var total int64
	for _, file := range files {
		total += file.size
	}
	if total <= s.opts.MaxTotalBytes {
		return nil
	}

	preferredCutoff := now.Add(-s.opts.PreferredMinAge)
	removed := make(map[string]bool)

	for _, preserveRecent := range []bool{true, false} {
		for _, file := range files {
			if total <= s.opts.MaxTotalBytes {
				return nil
			}
			if file.path == currentPath || removed[file.path] {
				continue
			}
			if preserveRecent && !file.modTime.Before(preferredCutoff) {
				continue
			}
			if !preserveRecent && file.modTime.Before(preferredCutoff) {
				continue
			}

			if err := os.Remove(file.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove oversized application log %q: %w", file.name, err)
			}
			removed[file.path] = true
			total -= file.size
		}
	}

	if total > s.opts.MaxTotalBytes {
		return fmt.Errorf(
			"application logs remain above hard limit: %d > %d bytes",
			total,
			s.opts.MaxTotalBytes,
		)
	}

	return nil
}

func (s *Store) listLogFiles() ([]logFileInfo, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("read application log directory: %w", err)
	}

	files := make([]logFileInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !isManagedLogName(entry.Name()) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("stat application log %q: %w", entry.Name(), err)
		}

		files = append(files, logFileInfo{
			path:    filepath.Join(s.dir, entry.Name()),
			name:    entry.Name(),
			size:    info.Size(),
			modTime: info.ModTime(),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].modTime.Equal(files[j].modTime) {
			return files[i].name < files[j].name
		}
		return files[i].modTime.Before(files[j].modTime)
	})

	return files, nil
}

func isManagedLogName(name string) bool {
	return strings.HasPrefix(name, "log-") && strings.HasSuffix(name, ".log")
}

// Close flushes and closes the active portable log file.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}

	s.archiveMu.Lock()
	defer s.archiveMu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	if s.file == nil {
		return nil
	}

	syncErr := s.file.Sync()
	closeErr := s.file.Close()
	s.file = nil

	return errors.Join(syncErr, closeErr)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
