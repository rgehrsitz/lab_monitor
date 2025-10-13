package report

import (
	"testing"
	"time"

	"labmonitor/internal/thingspeak"
)

func TestAggregator_Summarize_EmptyFeeds(t *testing.T) {
	agg := NewAggregator()

	summary := agg.Summarize(
		[]thingspeak.Feed{},
		12345,
		"field1",
		"field2",
		24*time.Hour,
		time.Now(),
		"Test Lab",
		"baseline 24h",
	)

	if summary.LabName != "Test Lab" {
		t.Errorf("Expected lab name 'Test Lab', got '%s'", summary.LabName)
	}
	if summary.Samples != 0 {
		t.Errorf("Expected 0 samples, got %d", summary.Samples)
	}
	if summary.WindowLabel != "1d" {
		t.Errorf("Expected window label '1d', got '%s'", summary.WindowLabel)
	}
}

func TestAggregator_Summarize_WithData(t *testing.T) {
	agg := NewAggregator()

	refTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	feeds := []thingspeak.Feed{
		{
			CreatedAt: refTime.Add(-23 * time.Hour),
			EntryID:   1,
			Fields: map[string]float64{
				"field1": 70.0, // temp
				"field2": 40.0, // humidity
			},
		},
		{
			CreatedAt: refTime.Add(-12 * time.Hour),
			EntryID:   2,
			Fields: map[string]float64{
				"field1": 75.0,
				"field2": 45.0,
			},
		},
		{
			CreatedAt: refTime.Add(-1 * time.Hour),
			EntryID:   3,
			Fields: map[string]float64{
				"field1": 80.0,
				"field2": 50.0,
			},
		},
	}

	summary := agg.Summarize(
		feeds,
		12345,
		"field1",
		"field2",
		24*time.Hour,
		refTime,
		"Test Lab",
		"baseline 24h",
	)

	// Check basic fields
	if summary.LabName != "Test Lab" {
		t.Errorf("Expected lab name 'Test Lab', got '%s'", summary.LabName)
	}
	if summary.Samples != 3 {
		t.Errorf("Expected 3 samples, got %d", summary.Samples)
	}
	if summary.ChannelID != 12345 {
		t.Errorf("Expected channel ID 12345, got %d", summary.ChannelID)
	}

	// Check temperature stats
	if summary.TemperatureStats.Min != 70.0 {
		t.Errorf("Expected temp min 70.0, got %f", summary.TemperatureStats.Min)
	}
	if summary.TemperatureStats.Max != 80.0 {
		t.Errorf("Expected temp max 80.0, got %f", summary.TemperatureStats.Max)
	}
	expectedMean := (70.0 + 75.0 + 80.0) / 3.0
	if summary.TemperatureStats.Mean != expectedMean {
		t.Errorf("Expected temp mean %f, got %f", expectedMean, summary.TemperatureStats.Mean)
	}
	expectedDelta := 80.0 - 70.0
	if summary.TemperatureStats.Delta != expectedDelta {
		t.Errorf("Expected temp delta %f, got %f", expectedDelta, summary.TemperatureStats.Delta)
	}

	// Check humidity stats
	if summary.HumidityStats.Min != 40.0 {
		t.Errorf("Expected humidity min 40.0, got %f", summary.HumidityStats.Min)
	}
	if summary.HumidityStats.Max != 50.0 {
		t.Errorf("Expected humidity max 50.0, got %f", summary.HumidityStats.Max)
	}

	// Check latest values
	if summary.LatestTemperature != 80.0 {
		t.Errorf("Expected latest temp 80.0, got %f", summary.LatestTemperature)
	}
	if summary.LatestHumidity != 50.0 {
		t.Errorf("Expected latest humidity 50.0, got %f", summary.LatestHumidity)
	}
}

func TestAggregator_Summarize_FiltersByWindow(t *testing.T) {
	agg := NewAggregator()

	refTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	feeds := []thingspeak.Feed{
		{
			CreatedAt: refTime.Add(-25 * time.Hour), // Outside 24h window
			EntryID:   1,
			Fields: map[string]float64{
				"field1": 60.0,
			},
		},
		{
			CreatedAt: refTime.Add(-12 * time.Hour), // Inside 24h window
			EntryID:   2,
			Fields: map[string]float64{
				"field1": 75.0,
			},
		},
		{
			CreatedAt: refTime.Add(-1 * time.Hour), // Inside 24h window
			EntryID:   3,
			Fields: map[string]float64{
				"field1": 80.0,
			},
		},
	}

	summary := agg.Summarize(
		feeds,
		12345,
		"field1",
		"field2",
		24*time.Hour,
		refTime,
		"Test Lab",
		"baseline 24h",
	)

	// Should only include 2 samples within the 24h window
	if summary.Samples != 2 {
		t.Errorf("Expected 2 samples within window, got %d", summary.Samples)
	}
	// Min should be 75.0, not 60.0
	if summary.TemperatureStats.Min != 75.0 {
		t.Errorf("Expected temp min 75.0 (60.0 should be filtered), got %f", summary.TemperatureStats.Min)
	}
}

func TestComputeStats_EmptyValues(t *testing.T) {
	feeds := []thingspeak.Feed{}
	stats := computeStats(feeds, "field1")

	if stats.Min != 0 || stats.Max != 0 || stats.Mean != 0 || stats.Delta != 0 {
		t.Errorf("Expected zero stats for empty feeds, got %+v", stats)
	}
}

func TestComputeStats_SingleValue(t *testing.T) {
	feeds := []thingspeak.Feed{
		{
			Fields: map[string]float64{"field1": 72.5},
		},
	}
	stats := computeStats(feeds, "field1")

	if stats.Min != 72.5 {
		t.Errorf("Expected min 72.5, got %f", stats.Min)
	}
	if stats.Max != 72.5 {
		t.Errorf("Expected max 72.5, got %f", stats.Max)
	}
	if stats.Mean != 72.5 {
		t.Errorf("Expected mean 72.5, got %f", stats.Mean)
	}
	if stats.Delta != 0 {
		t.Errorf("Expected delta 0, got %f", stats.Delta)
	}
}

func TestFormatWindow(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{24 * time.Hour, "1d"},
		{48 * time.Hour, "2d"},
		{12 * time.Hour, "12h"},
		{1 * time.Hour, "1h"},
		{30 * time.Minute, "30m"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatWindow(tt.duration)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
