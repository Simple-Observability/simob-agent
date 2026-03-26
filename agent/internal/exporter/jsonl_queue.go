package exporter

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"agent/internal/logger"
)

const (
	// maxLineSize bounds the memory spent on a single queue entry while reading.
	// JSONL keeps one payload per line, so an abnormally large line usually means
	// corrupted input or a partial write. Dropping it is safer than letting one
	// bad record make the flusher allocate unbounded memory.
	maxLineSize = 1024 * 1024 // 1MB

	// lockStaleAfter defines when a lock file is considered abandoned and can be
	// reclaimed by another process.
	lockStaleAfter = 10 * time.Second

	// lockRetryDelay is the backoff used while another process owns the queue.
	lockRetryDelay = 100 * time.Millisecond
)

// jsonlQueue is a minimal persistent queue backed by a single JSONL file.
// Multiple processes can append to the same queue directory; coordination is
// done with a lock file so reads and writes stay serialized.
type jsonlQueue struct {
	name     string
	path     string
	tempPath string
	lockPath string
}

// newJSONLQueue builds the file paths for a queue stream.
func newJSONLQueue(name, dir string) *jsonlQueue {
	base := filepath.Join(dir, name)
	return &jsonlQueue{
		name:     name,
		path:     base + ".jsonl",
		tempPath: base + ".tmp",
		lockPath: base + ".lock",
	}
}

// Append adds one serialized payload as a single JSONL line.
func (q *jsonlQueue) Append(data []byte) error {
	unlock, err := q.lock()
	if err != nil {
		return err
	}
	defer unlock()

	file, err := os.OpenFile(q.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o660)
	if err != nil {
		return fmt.Errorf("open queue file %s: %w", q.name, err)
	}
	defer file.Close()

	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("append to queue %s: %w", q.name, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync queue %s: %w", q.name, err)
	}
	return nil
}

// PopBatch drains up to limit entries and rewrites any remainder back to disk.
// The method holds the queue lock for the whole operation so multiple writers
// and the single flusher never observe a partially rewritten file.
func (q *jsonlQueue) PopBatch(limit int) ([][]byte, bool, error) {
	unlock, err := q.lock()
	if err != nil {
		return nil, false, err
	}
	defer unlock()

	source, err := os.OpenFile(q.path, os.O_CREATE|os.O_RDONLY, 0o660)
	if err != nil {
		return nil, false, fmt.Errorf("open queue file %s: %w", q.name, err)
	}
	defer source.Close()

	temp, err := os.OpenFile(q.tempPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o660)
	if err != nil {
		return nil, false, fmt.Errorf("open temp queue file %s: %w", q.name, err)
	}

	reader := bufio.NewReader(source)
	var batch [][]byte
	hasMore := false
	var leftoverBytes int64
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if len(line) > maxLineSize {
				logger.Log.Error("dropping oversized spool entry", "queue", q.name, "size", len(line))
				continue
			}
			line = trimTrailingNewline(line)
			if len(line) == 0 {
				continue
			}
			if len(batch) < limit {
				batch = append(batch, append([]byte(nil), line...))
			} else {
				written, writeErr := temp.Write(append(line, '\n'))
				if writeErr != nil {
					temp.Close()
					return nil, false, fmt.Errorf("rewrite queue %s: %w", q.name, writeErr)
				}
				leftoverBytes += int64(written)
				hasMore = true
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			temp.Close()
			return nil, false, fmt.Errorf("read queue %s: %w", q.name, err)
		}
	}

	if err := temp.Close(); err != nil {
		return nil, false, fmt.Errorf("close temp queue %s: %w", q.name, err)
	}

	if err := os.Remove(q.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, false, fmt.Errorf("replace queue %s: %w", q.name, err)
	}
	if leftoverBytes == 0 {
		if err := os.Remove(q.tempPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, false, fmt.Errorf("cleanup temp queue %s: %w", q.name, err)
		}
		return batch, false, nil
	}
	if err := os.Rename(q.tempPath, q.path); err != nil {
		return nil, false, fmt.Errorf("replace queue %s: %w", q.name, err)
	}
	return batch, hasMore, nil
}

// Close exists so spool can treat all queue implementations uniformly.
func (q *jsonlQueue) Close() error {
	return nil
}

// lock acquires exclusive ownership of the queue by creating a lock file.
// If the owner disappears without removing it, the lock is reclaimed after
// lockStaleAfter.
func (q *jsonlQueue) lock() (func(), error) {
	for {
		lockFile, err := os.OpenFile(q.lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o660)
		if err == nil {
			_, _ = fmt.Fprintf(lockFile, "%d\n", os.Getpid())
			_ = lockFile.Close()
			return func() {
				if err := os.Remove(q.lockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
					logger.Log.Error("failed to release spool lock", "queue", q.name, "error", err)
				}
			}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("acquire queue lock %s: %w", q.name, err)
		}

		info, statErr := os.Stat(q.lockPath)
		if statErr == nil && time.Since(info.ModTime()) > lockStaleAfter {
			if removeErr := os.Remove(q.lockPath); removeErr == nil || errors.Is(removeErr, os.ErrNotExist) {
				continue
			}
		}
		time.Sleep(lockRetryDelay)
	}
}

// trimTrailingNewline normalizes lines read from the JSONL file before they are
// passed to the payload unmarshal step.
func trimTrailingNewline(line []byte) []byte {
	if n := len(line); n > 0 && line[n-1] == '\n' {
		line = line[:n-1]
	}
	if n := len(line); n > 0 && line[n-1] == '\r' {
		line = line[:n-1]
	}
	return line
}
