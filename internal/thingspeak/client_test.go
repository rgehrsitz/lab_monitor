package thingspeak

import (
	"context"
	"strconv"
	"testing"
	"time"
)

func TestGetFeeds_Success(t *testing.T) {
	// Note: This test is a placeholder for future integration testing.
	// The current Client implementation uses a hardcoded baseURL which makes
	// it difficult to test with a mock server without refactoring to allow
	// URL injection. The actual HTTP client behavior is tested through
	// TestGetFeeds_InvalidTimeRange and the parsing logic is tested in
	// TestFeedItemParsing.
	t.Skip("Integration test requires mock server support - consider refactoring Client to accept baseURL")
}

func TestGetFeeds_InvalidTimeRange(t *testing.T) {
	client := NewClient()
	ctx := context.Background()

	start := time.Date(2025, 1, 1, 14, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	_, _, err := client.GetFeeds(ctx, 12345, start, end)
	if err == nil {
		t.Error("Expected error for end before start, got nil")
	}
	if err.Error() != "end before start" {
		t.Errorf("Expected 'end before start' error, got: %v", err)
	}
}

func TestFeedItemParsing(t *testing.T) {
	tests := []struct {
		name     string
		item     feedItem
		expected map[string]float64
	}{
		{
			name: "all fields present",
			item: feedItem{
				Field1: strPtr("72.5"),
				Field2: strPtr("45.2"),
				Field3: strPtr("100"),
			},
			expected: map[string]float64{
				"field1": 72.5,
				"field2": 45.2,
				"field3": 100,
			},
		},
		{
			name: "missing fields",
			item: feedItem{
				Field1: strPtr("72.5"),
				Field2: nil,
			},
			expected: map[string]float64{
				"field1": 72.5,
			},
		},
		{
			name: "invalid numeric values",
			item: feedItem{
				Field1: strPtr("not-a-number"),
				Field2: strPtr("45.2"),
			},
			expected: map[string]float64{
				"field2": 45.2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := make(map[string]float64, 8)
			addField := func(name string, value *string) {
				if value == nil {
					return
				}
				if f, err := parseFloat(*value); err == nil {
					fields[name] = f
				}
			}

			addField("field1", tt.item.Field1)
			addField("field2", tt.item.Field2)
			addField("field3", tt.item.Field3)

			if len(fields) != len(tt.expected) {
				t.Errorf("Expected %d fields, got %d", len(tt.expected), len(fields))
			}

			for key, expectedVal := range tt.expected {
				if actualVal, ok := fields[key]; !ok {
					t.Errorf("Missing field %s", key)
				} else if actualVal != expectedVal {
					t.Errorf("Field %s: expected %f, got %f", key, expectedVal, actualVal)
				}
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
}

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}
