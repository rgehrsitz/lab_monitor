package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewStore_CreatesDirectory(t *testing.T) {
	tempDir := t.TempDir()
	stateDir := filepath.Join(tempDir, "state")

	store, err := NewStore(stateDir, 5)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	if store == nil {
		t.Fatal("Expected non-nil store")
	}

	// Verify directory was created
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		t.Error("Expected state directory to be created")
	}
}

func TestStore_SaveAndLoad_SingleReport(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewStore(tempDir, 5)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create test report
	report := ReportRecord{
		Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		Overall:   "normal",
		PerLab: []LabReport{
			{
				LabName:   "Lab A",
				Summary:   "All good",
				Status:    "normal",
				Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			},
		},
		RawResponse:  `{"status":"normal"}`,
		Escalations:  map[string]int{"Lab A": 24},
		PromptDigest: "abc123",
	}

	// Save report
	if err := store.SaveReport(report); err != nil {
		t.Fatalf("Failed to save report: %v", err)
	}

	// Load history
	history, err := store.LoadHistory()
	if err != nil {
		t.Fatalf("Failed to load history: %v", err)
	}

	if len(history) != 1 {
		t.Fatalf("Expected 1 report in history, got %d", len(history))
	}

	loaded := history[0]
	if loaded.Overall != "normal" {
		t.Errorf("Expected overall status 'normal', got '%s'", loaded.Overall)
	}
	if len(loaded.PerLab) != 1 {
		t.Errorf("Expected 1 lab report, got %d", len(loaded.PerLab))
	}
	if loaded.PerLab[0].LabName != "Lab A" {
		t.Errorf("Expected lab name 'Lab A', got '%s'", loaded.PerLab[0].LabName)
	}
}

func TestStore_SaveReport_TrimHistory(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewStore(tempDir, 3) // Only keep 3 reports
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Save 5 reports
	for i := 1; i <= 5; i++ {
		report := ReportRecord{
			Timestamp: time.Date(2025, 1, 1, 12, i, 0, 0, time.UTC),
			Overall:   "normal",
			PerLab: []LabReport{
				{
					LabName:   "Lab A",
					Summary:   "Report " + string(rune('0'+i)),
					Status:    "normal",
					Timestamp: time.Date(2025, 1, 1, 12, i, 0, 0, time.UTC),
				},
			},
			RawResponse:  `{"status":"normal"}`,
			PromptDigest: "abc123",
		}
		if err := store.SaveReport(report); err != nil {
			t.Fatalf("Failed to save report %d: %v", i, err)
		}
	}

	// Load history - should only have 3 most recent
	history, err := store.LoadHistory()
	if err != nil {
		t.Fatalf("Failed to load history: %v", err)
	}

	if len(history) != 3 {
		t.Fatalf("Expected 3 reports after trimming, got %d", len(history))
	}

	// Verify we have the most recent 3 (reports 3, 4, 5)
	if history[0].Timestamp.Minute() != 3 {
		t.Errorf("Expected first report to be minute 3, got %d", history[0].Timestamp.Minute())
	}
	if history[2].Timestamp.Minute() != 5 {
		t.Errorf("Expected last report to be minute 5, got %d", history[2].Timestamp.Minute())
	}
}

func TestStore_LatestReport_Empty(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewStore(tempDir, 5)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	latest, err := store.LatestReport()
	if err != nil {
		t.Fatalf("Expected no error for empty history, got: %v", err)
	}
	if latest != nil {
		t.Error("Expected nil for empty history")
	}
}

func TestStore_LatestReport_WithData(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewStore(tempDir, 5)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Save 3 reports
	for i := 1; i <= 3; i++ {
		report := ReportRecord{
			Timestamp: time.Date(2025, 1, 1, 12, i, 0, 0, time.UTC),
			Overall:   "normal",
			PerLab: []LabReport{
				{
					LabName:   "Lab A",
					Summary:   "Report " + string(rune('0'+i)),
					Status:    "normal",
					Timestamp: time.Date(2025, 1, 1, 12, i, 0, 0, time.UTC),
				},
			},
			RawResponse:  `{"status":"normal"}`,
			PromptDigest: "abc" + string(rune('0'+i)),
		}
		if err := store.SaveReport(report); err != nil {
			t.Fatalf("Failed to save report %d: %v", i, err)
		}
	}

	latest, err := store.LatestReport()
	if err != nil {
		t.Fatalf("Failed to get latest report: %v", err)
	}
	if latest == nil {
		t.Fatal("Expected non-nil latest report")
	}

	// Should be report 3
	if latest.Timestamp.Minute() != 3 {
		t.Errorf("Expected latest report minute 3, got %d", latest.Timestamp.Minute())
	}
	if latest.PromptDigest != "abc3" {
		t.Errorf("Expected prompt digest 'abc3', got '%s'", latest.PromptDigest)
	}
}

func TestTrimLimit_NoTrim(t *testing.T) {
	records := []ReportRecord{
		{Timestamp: time.Now()},
		{Timestamp: time.Now()},
	}

	result := trimLimit(records, 5)
	if len(result) != 2 {
		t.Errorf("Expected 2 records, got %d", len(result))
	}
}

func TestTrimLimit_WithTrim(t *testing.T) {
	records := []ReportRecord{
		{Timestamp: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)},
		{Timestamp: time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC)},
		{Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)},
		{Timestamp: time.Date(2025, 1, 1, 13, 0, 0, 0, time.UTC)},
		{Timestamp: time.Date(2025, 1, 1, 14, 0, 0, 0, time.UTC)},
	}

	result := trimLimit(records, 3)
	if len(result) != 3 {
		t.Errorf("Expected 3 records, got %d", len(result))
	}

	// Should keep the last 3 (12:00, 13:00, 14:00)
	if result[0].Timestamp.Hour() != 12 {
		t.Errorf("Expected first record at 12:00, got %d:00", result[0].Timestamp.Hour())
	}
	if result[2].Timestamp.Hour() != 14 {
		t.Errorf("Expected last record at 14:00, got %d:00", result[2].Timestamp.Hour())
	}
}

func TestTrimLimit_ZeroLimit(t *testing.T) {
	records := []ReportRecord{
		{Timestamp: time.Now()},
		{Timestamp: time.Now()},
	}

	result := trimLimit(records, 0)
	if len(result) != 2 {
		t.Errorf("Expected no trimming with zero limit, got %d records", len(result))
	}
}

func TestStore_LoadHistory_NonExistentFile(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewStore(tempDir, 5)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Should return empty slice, not error
	history, err := store.LoadHistory()
	if err != nil {
		t.Errorf("Expected no error for non-existent history file, got: %v", err)
	}
	if history != nil {
		t.Errorf("Expected nil history for non-existent file, got %d records", len(history))
	}
}
