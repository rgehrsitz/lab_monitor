package report

import (
	"fmt"
	"sort"
	"time"

	"labmonitor/internal/thingspeak"
)

// Measurement summary for a lab.
type Summary struct {
	LabName           string    `json:"lab_name"`
	ChannelID         int       `json:"channel_id"`
	WindowLabel       string    `json:"window"`
	WindowHours       int       `json:"window_hours"`
	Start             time.Time `json:"start"`
	End               time.Time `json:"end"`
	TemperatureStats  Stats     `json:"temperature"`
	HumidityStats     Stats     `json:"humidity"`
	LatestTemperature float64   `json:"latest_temperature"`
	LatestHumidity    float64   `json:"latest_humidity"`
	Samples           int       `json:"samples"`
	Context           string    `json:"context"`
}

func formatWindow(d time.Duration) string {
	hours := int(d.Hours())
	if hours >= 24 && hours%24 == 0 {
		days := hours / 24
		if days == 1 {
			return "1d"
		}
		return fmt.Sprintf("%dd", days)
	}
	if hours >= 1 {
		return fmt.Sprintf("%dh", hours)
	}
	minutes := int(d.Minutes())
	if minutes >= 1 {
		return fmt.Sprintf("%dm", minutes)
	}
	return d.String()
}

type Stats struct {
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Mean  float64 `json:"mean"`
	Delta float64 `json:"delta"`
}

type Aggregator struct{}

func NewAggregator() Aggregator {
	return Aggregator{}
}

func (Aggregator) Summarize(feeds []thingspeak.Feed, channelID int, tempField, humidityField string, window time.Duration, refTime time.Time, labName string, contextLabel string) Summary {
	// Filter by window
	cutoff := refTime.Add(-window)
	filtered := make([]thingspeak.Feed, 0, len(feeds))
	for _, f := range feeds {
		if f.CreatedAt.After(cutoff) {
			filtered = append(filtered, f)
		}
	}
	feeds = filtered
	windowHours := int(window.Hours())
	windowLabel := formatWindow(window)
	if len(feeds) == 0 {
		return Summary{LabName: labName, ChannelID: channelID, WindowLabel: windowLabel, WindowHours: windowHours, Context: contextLabel}
	}

	sort.Slice(feeds, func(i, j int) bool {
		return feeds[i].CreatedAt.Before(feeds[j].CreatedAt)
	})

	tStats := computeStats(feeds, tempField)
	hStats := computeStats(feeds, humidityField)

	latest := feeds[len(feeds)-1]
	latestTemp, _ := latest.Fields[tempField]
	latestHumidity, _ := latest.Fields[humidityField]

	return Summary{
		LabName:           labName,
		ChannelID:         channelID,
		WindowLabel:       windowLabel,
		WindowHours:       windowHours,
		Start:             feeds[0].CreatedAt,
		End:               feeds[len(feeds)-1].CreatedAt,
		TemperatureStats:  tStats,
		HumidityStats:     hStats,
		LatestTemperature: latestTemp,
		LatestHumidity:    latestHumidity,
		Samples:           len(feeds),
		Context:           contextLabel,
	}
}

func computeStats(feeds []thingspeak.Feed, field string) Stats {
	values := make([]float64, 0, len(feeds))
	for _, f := range feeds {
		v, ok := f.Fields[field]
		if !ok {
			continue
		}
		values = append(values, v)
	}
	if len(values) == 0 {
		return Stats{}
	}
	min := values[0]
	max := values[0]
	sum := 0.0
	for _, v := range values {
		sum += v
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return Stats{
		Min:   min,
		Max:   max,
		Mean:  sum / float64(len(values)),
		Delta: values[len(values)-1] - values[0],
	}
}
