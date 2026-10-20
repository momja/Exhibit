package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/momja/Exhibit/internal/agentcontrol"
	"github.com/momja/Exhibit/internal/secrets"
	"github.com/momja/Exhibit/internal/sprites"
)

// The journal is coordinator state, not worker metadata. Encrypt the complete
// record to authenticate identity as well as credentials, under a separate key.
// A request is durable before the first management call; uncertain retries and
// restart cleanup therefore use the original identity and endpoint credential.
type workerRecord struct {
	OwnerID    int64
	Prefix     string
	EraseOwner bool
	Assignment agentcontrol.Assignment
	Request    sprites.Request
	IdleSince  time.Time
}

type workerJournal struct {
	mu      sync.Mutex
	path    string
	box     *secrets.Box
	records map[string]workerRecord
}

func openWorkerJournal(dataDir string, box *secrets.Box) (*workerJournal, error) {
	if box == nil {
		return nil, errors.New("worker journal requires server secrets")
	}
	key := box.DeriveKey("exhibit/agent-worker-journal/v1")
	journalBox, err := secrets.Load(hex.EncodeToString(key[:]), "")
	if err != nil {
		return nil, err
	}
	directory := filepath.Join(dataDir, "agent-workers")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(directory, 0700); err != nil {
		return nil, err
	}
	j := &workerJournal{path: filepath.Join(directory, "assignments.enc"), box: journalBox, records: map[string]workerRecord{}}
	info, err := os.Stat(j.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil && (!info.Mode().IsRegular() || info.Size() > 16<<20) {
		return nil, errors.New("invalid or oversized worker journal")
	}
	data, err := os.ReadFile(j.path)
	if errors.Is(err, os.ErrNotExist) {
		return j, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) > 16<<20 {
		return nil, errors.New("worker journal exceeds limit")
	}
	plaintext, err := j.box.Decrypt(string(data))
	if err != nil {
		return nil, errors.New("cannot authenticate worker journal")
	}
	if err := json.Unmarshal([]byte(plaintext), &j.records); err != nil {
		return nil, errors.New("invalid worker journal")
	}
	for id, record := range j.records {
		record.Assignment.OwnerID = record.OwnerID
		j.records[id] = record
	}
	if j.records == nil {
		j.records = map[string]workerRecord{}
	}
	return j, nil
}

func (j *workerJournal) snapshot() map[string]workerRecord {
	j.mu.Lock()
	defer j.mu.Unlock()
	records := make(map[string]workerRecord, len(j.records))
	for id, record := range j.records {
		records[id] = record
	}
	return records
}

func (j *workerJournal) update(id string, record *workerRecord) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	previous, existed := j.records[id]
	if record == nil {
		delete(j.records, id)
	} else {
		j.records[id] = *record
	}
	if err := j.persist(); err != nil {
		if existed {
			j.records[id] = previous
		} else {
			delete(j.records, id)
		}
		return err
	}
	return nil
}

func (j *workerJournal) persist() error {
	data, err := json.Marshal(j.records)
	if err != nil {
		return err
	}
	encrypted, err := j.box.Encrypt(string(data))
	if err != nil {
		return err
	}
	directory := filepath.Dir(j.path)
	file, err := os.CreateTemp(directory, ".assignments-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0600); err == nil {
		_, err = file.WriteString(encrypted)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(file.Name(), j.path); err != nil {
		return err
	}
	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync worker journal directory: %w", err)
	}
	return nil
}
