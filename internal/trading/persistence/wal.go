// WAL (Write-Ahead Log) for durability and crash recovery.
// All write requests are appended before DB flush.
package persistence

import (
	"encoding/json"
	"io"
	"os"
	"sync"
)

type WAL interface {
	Append(req WriteRequest) (uint64, error)
	ReadAll() ([]WriteRequest, error)
	Truncate(uptoSeq uint64) error
}

type FileWAL struct {
	file *os.File
	mu   sync.Mutex
	seq  uint64
}

func NewFileWAL(path string) (*FileWAL, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	w := &FileWAL{file: f, seq: 0}
	// Recover last seq from file
	if err := w.recoverSeq(); err != nil {
		return nil, err
	}
	return w, nil
}

// Append serializes and writes a WriteRequest to the WAL file, returns the new sequence number.
func (w *FileWAL) Append(req WriteRequest) (uint64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.seq++
	req.WALSeq = w.seq
	b, err := json.Marshal(req)
	if err != nil {
		return 0, err
	}
	b = append(b, '\n')
	if _, err := w.file.Write(b); err != nil {
		return 0, err
	}
	if err := w.file.Sync(); err != nil {
		return 0, err
	}
	return w.seq, nil
}

// ReadAll replays all WriteRequests from the WAL file.
func (w *FileWAL) ReadAll() ([]WriteRequest, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	var reqs []WriteRequest
	dec := json.NewDecoder(w.file)
	for {
		var req WriteRequest
		err := dec.Decode(&req)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		reqs = append(reqs, req)
	}
	return reqs, nil
}

// Truncate removes WAL entries up to (and including) uptoSeq by rewriting the file.
func (w *FileWAL) Truncate(uptoSeq uint64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	var keep []WriteRequest
	dec := json.NewDecoder(w.file)
	for {
		var req WriteRequest
		err := dec.Decode(&req)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if req.WALSeq > uptoSeq {
			keep = append(keep, req)
		}
	}
	// Rewrite file
	if err := w.file.Truncate(0); err != nil {
		return err
	}
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	for _, req := range keep {
		b, err := json.Marshal(req)
		if err != nil {
			return err
		}
		b = append(b, '\n')
		if _, err := w.file.Write(b); err != nil {
			return err
		}
	}
	return w.file.Sync()
}

// recoverSeq scans the WAL file to find the last sequence number.
func (w *FileWAL) recoverSeq() error {
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	var maxSeq uint64
	dec := json.NewDecoder(w.file)
	for {
		var req WriteRequest
		err := dec.Decode(&req)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if req.WALSeq > maxSeq {
			maxSeq = req.WALSeq
		}
	}
	w.seq = maxSeq
	return nil
}
