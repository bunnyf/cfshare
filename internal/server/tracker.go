package server

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"cfshare/internal/config"
	"cfshare/internal/state"
)

type transferEntry struct {
	id         string
	fileName   string
	totalSize  int64
	rangeStart int64
	startTime  time.Time
	remoteAddr string

	bytesSent  int64 // accessed via sync/atomic, hot path

	mu         sync.Mutex // protects done + finishedAt
	done       bool
	finishedAt time.Time
}

// TransferTracker 管理活动的文件传输状态，每 250ms 将快照持久化到 transfers.json
type TransferTracker struct {
	mu      sync.RWMutex
	entries map[string]*transferEntry
	seq     int64 // atomic counter for IDs
}

func newTransferTracker() *TransferTracker {
	t := &TransferTracker{
		entries: make(map[string]*transferEntry),
	}
	go t.flushLoop()
	return t
}

// Start 登记一次新传输并返回传输 ID
func (t *TransferTracker) Start(fileName string, totalSize, rangeStart int64, remoteAddr string) string {
	id := fmt.Sprintf("%d", atomic.AddInt64(&t.seq, 1))
	e := &transferEntry{
		id:         id,
		fileName:   fileName,
		totalSize:  totalSize,
		rangeStart: rangeStart,
		startTime:  time.Now(),
		remoteAddr: remoteAddr,
	}
	t.mu.Lock()
	t.entries[id] = e
	t.mu.Unlock()
	return id
}

// Add 原子增加已传字节数（热路径，仅对 map 短暂加读锁）
func (t *TransferTracker) Add(id string, n int64) {
	t.mu.RLock()
	e := t.entries[id]
	t.mu.RUnlock()
	if e != nil {
		atomic.AddInt64(&e.bytesSent, n)
	}
}

// Finish 标记传输结束并立即落盘一次
func (t *TransferTracker) Finish(id string) {
	t.mu.RLock()
	e := t.entries[id]
	t.mu.RUnlock()
	if e != nil {
		e.mu.Lock()
		e.done = true
		e.finishedAt = time.Now()
		e.mu.Unlock()
	}
	t.flush()
}

func (t *TransferTracker) snapshot() []state.TransferRecord {
	t.mu.RLock()
	defer t.mu.RUnlock()

	records := make([]state.TransferRecord, 0, len(t.entries))
	for _, e := range t.entries {
		e.mu.Lock()
		done := e.done
		e.mu.Unlock()
		records = append(records, state.TransferRecord{
			ID:         e.id,
			FileName:   e.fileName,
			TotalSize:  e.totalSize,
			BytesSent:  atomic.LoadInt64(&e.bytesSent),
			RangeStart: e.rangeStart,
			StartTime:  e.startTime,
			RemoteAddr: e.remoteAddr,
			Done:       done,
		})
	}
	return records
}

func (t *TransferTracker) cleanup() {
	deadline := time.Now().Add(-5 * time.Second)
	t.mu.Lock()
	for id, e := range t.entries {
		e.mu.Lock()
		done := e.done
		finished := e.finishedAt
		e.mu.Unlock()
		if done && finished.Before(deadline) {
			delete(t.entries, id)
		}
	}
	t.mu.Unlock()
}

func (t *TransferTracker) flush() {
	snap := state.TransferSnapshot{
		Transfers: t.snapshot(),
		UpdatedAt: time.Now(),
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return
	}
	os.WriteFile(config.GetTransfersPath(), data, 0600)
}

func (t *TransferTracker) flushLoop() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		t.flush()
		t.cleanup()
	}
}
