package server

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
)

// auditHash chains an entry onto the previous entry's hash, covering EVERY
// persisted field — including the display time, so even a cosmetic-looking
// edit of a line is detected. Any edit, drop or reorder of a persisted entry
// changes every hash after it, so a verifier only needs the file itself to
// prove integrity (the chain is self-anchoring from the genesis entry; pair
// it with an out-of-band copy of the final hash to also detect truncation).
func auditHash(prev string, e LogEntry) string {
	h := sha256.Sum256([]byte(prev + "|" + strconv.Itoa(e.Seq) + "|" + e.Time + "|" + e.TS + "|" + e.Source + "|" + e.Detail))
	return hex.EncodeToString(h[:])
}

// SetAuditLog appends every activity entry to path as hash-chained JSONL.
// Call before the server starts handling requests.
func (s *Server) SetAuditLog(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	// Resume the chain when appending to an existing log, so one file spans
	// server restarts and still verifies end-to-end.
	if last, n, err := lastAuditEntry(path); err == nil && n > 0 {
		s.actMu.Lock()
		s.lastHash = last.Hash
		s.actSeq = last.Seq
		s.actMu.Unlock()
	}
	s.actMu.Lock()
	s.auditW = f
	s.actMu.Unlock()
	return nil
}

// lastAuditEntry reads path and returns its final entry and line count.
func lastAuditEntry(path string) (LogEntry, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return LogEntry{}, 0, err
	}
	defer f.Close()
	var last LogEntry
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var e LogEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return LogEntry{}, n, fmt.Errorf("line %d: %w", n+1, err)
		}
		last = e
		n++
	}
	return last, n, sc.Err()
}

// VerifyAuditChain checks a JSONL audit log's hash chain and returns the
// number of verified entries. It fails on any edited, dropped, reordered or
// re-attributed entry.
func VerifyAuditChain(r io.Reader) (int, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	prev, n := "", 0
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var e LogEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return n, fmt.Errorf("entry %d: not valid JSON: %w", n+1, err)
		}
		if want := auditHash(prev, e); e.Hash != want {
			return n, fmt.Errorf("entry %d (seq %d, %s %q): hash mismatch — the log was modified here or an earlier entry was altered", n+1, e.Seq, e.Source, e.Detail)
		}
		prev = e.Hash
		n++
	}
	if err := sc.Err(); err != nil {
		return n, err
	}
	return n, nil
}
