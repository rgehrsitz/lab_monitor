package report

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"labmonitor/internal/state"
)

type PromptBuilder struct{}

func NewPromptBuilder() PromptBuilder {
	return PromptBuilder{}
}

type promptPayload struct {
	Timestamp        time.Time           `json:"timestamp"`
	Notes            []string            `json:"notes,omitempty"`
	CurrentSummaries []Summary           `json:"current_summaries"`
	PreviousReport   *state.ReportRecord `json:"previous_report,omitempty"`
	Instructions     string              `json:"instructions"`
}

func (PromptBuilder) Build(summaries []Summary, last *state.ReportRecord, notes []string) (string, error) {
	payload := promptPayload{
		Timestamp:        time.Now().UTC(),
		Notes:            notes,
		CurrentSummaries: summaries,
		PreviousReport:   last,
		Instructions: strings.Join([]string{
			"You are monitoring temperature and humidity for multiple labs.",
			"Review the JSON summaries and produce a JSON response matching this schema: {\"status\": string, \"summary\": string, \"labs\": [{\"name\": string, \"status\": string, \"details\": string, \"recommendations\": [string]}], \"recommendations\": [string], \"need_context\": {\"labs\": [{\"name\": string, \"window_hours\": number, \"reason\": string}]} }.",
			"Acceptable status values: normal, watch, alert.",
			"If you require more historical context, populate need_context with specific lab names and exact window_hours you require (integer hours), plus a concise reason. Do not include other content when requesting context.",
			"Historical data requests can span arbitrary windows; the system will stitch together multiple API slices (each capped at roughly 8000 samples), so ask only for the smallest window that unblocks your analysis.",
			"Otherwise, provide actionable analysis covering notable trends, spikes, comfort issues, and recommendations. Prefer concise sentences.",
			"Always return valid JSON without markdown fences or commentary.",
		}, "\n"),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal prompt: %w", err)
	}
	return string(data), nil
}
