package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type LabReport struct {
	LabName   string    `json:"lab_name"`
	Summary   string    `json:"summary"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

type ReportRecord struct {
	Timestamp    time.Time      `json:"timestamp"`
	Overall      string         `json:"overall"`
	PerLab       []LabReport    `json:"per_lab"`
	RawResponse  string         `json:"raw_response"`
	Escalations  map[string]int `json:"escalations"`
	PromptDigest string         `json:"prompt_digest"`
}

type Store struct {
	dir           string
	historyPerLab int
	mu            sync.Mutex
	combinedPath  string
}

func NewStore(dir string, historyPerLab int) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	return &Store{
		dir:           dir,
		historyPerLab: historyPerLab,
		combinedPath:  filepath.Join(dir, "history.json"),
	}, nil
}

func (s *Store) LoadHistory() ([]ReportRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.combinedPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read history: %w", err)
	}
	var records []ReportRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("parse history: %w", err)
	}
	return records, nil
}

func (s *Store) SaveReport(record ReportRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.loadHistoryLocked()
	if err != nil {
		return err
	}
	records = append(records, record)
	records = trimLimit(records, s.historyPerLab)

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("encode history: %w", err)
	}
	if err := os.WriteFile(s.combinedPath, data, 0o644); err != nil {
		return fmt.Errorf("write history: %w", err)
	}
	return nil
}

func (s *Store) loadHistoryLocked() ([]ReportRecord, error) {
	data, err := os.ReadFile(s.combinedPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read history: %w", err)
	}
	var records []ReportRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("parse history: %w", err)
	}
	return records, nil
}

func trimLimit(records []ReportRecord, limit int) []ReportRecord {
	if limit <= 0 {
		return records
	}
	if len(records) <= limit {
		return records
	}
	return append([]ReportRecord(nil), records[len(records)-limit:]...)
}

func (s *Store) LatestReport() (*ReportRecord, error) {
	records, err := s.LoadHistory()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	last := records[len(records)-1]
	return &last, nil
}
