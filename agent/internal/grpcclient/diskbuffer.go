package grpcclient

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	qfv1 "github.com/qf/qf/proto/qf/v1"
	"google.golang.org/protobuf/proto"
)

const (
	DefaultDiskBufDir  = "/var/lib/qf/events"
	diskBufMaxBytes    = 100 * 1024 * 1024 // 100 MB
)

// DiskBuffer persists pending AgentMessage protos to disk so they survive
// a CP disconnect. On reconnect, call Replay to drain the buffer.
// Files are named <unix_ns>.pb; oldest are dropped when the 100 MB cap is exceeded.
type DiskBuffer struct {
	dir string
}

// NewDiskBuffer creates a DiskBuffer rooted at dir.
// The directory is created on first Write if it does not exist.
func NewDiskBuffer(dir string) *DiskBuffer {
	if dir == "" {
		dir = DefaultDiskBufDir
	}
	return &DiskBuffer{dir: dir}
}

// Write serialises msg and appends it to the buffer.
// If the buffer exceeds 100 MB after writing, the oldest files are dropped.
func (db *DiskBuffer) Write(msg *qfv1.AgentMessage) error {
	if err := os.MkdirAll(db.dir, 0o750); err != nil {
		return fmt.Errorf("diskbuffer mkdir: %w", err)
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("diskbuffer marshal: %w", err)
	}

	name := fmt.Sprintf("%d.pb", time.Now().UnixNano())
	path := filepath.Join(db.dir, name)
	if err := os.WriteFile(path, data, 0o640); err != nil {
		return fmt.Errorf("diskbuffer write: %w", err)
	}

	db.evict()
	return nil
}

// Replay reads all buffered messages in chronological order and calls sendFn
// for each. Successfully sent messages are deleted. On ctx cancellation or
// sendFn error the replay stops.
func (db *DiskBuffer) Replay(ctx context.Context, sendFn func(*qfv1.AgentMessage) error) error {
	files, err := db.sortedFiles()
	if err != nil {
		return nil // nothing to replay
	}

	for _, f := range files {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		path := filepath.Join(db.dir, f)
		data, err := os.ReadFile(path)
		if err != nil {
			// File disappeared between listing and reading — skip.
			continue
		}

		var msg qfv1.AgentMessage
		if err := proto.Unmarshal(data, &msg); err != nil {
			// Corrupt file — delete and continue.
			os.Remove(path)
			continue
		}

		if err := sendFn(&msg); err != nil {
			return err
		}
		os.Remove(path)
	}
	return nil
}

// evict removes the oldest files until total size is below diskBufMaxBytes.
func (db *DiskBuffer) evict() {
	files, err := db.sortedFiles()
	if err != nil {
		return
	}

	var total int64
	sizes := make([]int64, len(files))
	for i, f := range files {
		info, err := os.Stat(filepath.Join(db.dir, f))
		if err == nil {
			sizes[i] = info.Size()
			total += sizes[i]
		}
	}

	for i, f := range files {
		if total <= diskBufMaxBytes {
			break
		}
		os.Remove(filepath.Join(db.dir, f))
		total -= sizes[i]
	}
}

// sortedFiles returns .pb files in chronological order (by unix_ns prefix).
func (db *DiskBuffer) sortedFiles() ([]string, error) {
	entries, err := os.ReadDir(db.dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".pb") {
			files = append(files, e.Name())
		}
	}

	sort.Slice(files, func(i, j int) bool {
		ni := pbNameToNs(files[i])
		nj := pbNameToNs(files[j])
		return ni < nj
	})
	return files, nil
}

func pbNameToNs(name string) int64 {
	base := strings.TrimSuffix(name, ".pb")
	n, _ := strconv.ParseInt(base, 10, 64)
	return n
}
