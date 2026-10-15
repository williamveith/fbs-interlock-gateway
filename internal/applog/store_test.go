package applog

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testOptions() Options {
	return Options{
		MaxAge:            30 * 24 * time.Hour,
		PreferredMinAge:   14 * 24 * time.Hour,
		MaxFileBytes:      128,
		MaxTotalBytes:     512,
		MaxRecordBytes:    64,
		CleanupEveryBytes: 64,
	}
}

func TestStoreWriteExportAndClose(t *testing.T) {
	store, err := Open(t.TempDir(), testOptions())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.Write([]byte("first line\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Write([]byte("second line\n")); err != nil {
		t.Fatal(err)
	}

	var exported bytes.Buffer
	if err := store.Export(context.Background(), &exported); err != nil {
		t.Fatal(err)
	}

	text := exported.String()
	if !strings.Contains(text, "first line\n") || !strings.Contains(text, "second line\n") {
		t.Fatalf("export missing log content:\n%s", text)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreRotatesAndBoundsRecords(t *testing.T) {
	dir := t.TempDir()
	opts := testOptions()
	opts.MaxFileBytes = 80
	opts.MaxRecordBytes = 64
	opts.CleanupEveryBytes = 16

	store, err := Open(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	longRecord := strings.Repeat("x", 100) + "\n"
	if _, err := store.Write([]byte(longRecord)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Write([]byte("another record\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Write([]byte("rotate now\n")); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected rotation to create at least two files, got %d", len(entries))
	}

	var exported bytes.Buffer
	if err := store.Export(context.Background(), &exported); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(exported.String(), "[application log record truncated]") {
		t.Fatalf("expected truncation marker in export:\n%s", exported.String())
	}
}

func TestCleanupHonorsHardTotalLimit(t *testing.T) {
	dir := t.TempDir()
	opts := testOptions()
	opts.MaxTotalBytes = 200
	opts.MaxFileBytes = 128

	now := time.Now()
	for i := 0; i < 4; i++ {
		path := filepath.Join(dir, "log-old-"+string(rune('a'+i))+".log")
		if err := os.WriteFile(path, bytes.Repeat([]byte{'x'}, 80), 0600); err != nil {
			t.Fatal(err)
		}
		stamp := now.Add(time.Duration(-20+i) * 24 * time.Hour)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}

	store, err := Open(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	files, err := store.listLogFiles()
	if err != nil {
		t.Fatal(err)
	}

	var total int64
	for _, file := range files {
		total += file.size
	}
	if total > opts.MaxTotalBytes {
		t.Fatalf("log archive exceeds hard limit: got %d want <= %d", total, opts.MaxTotalBytes)
	}
}
