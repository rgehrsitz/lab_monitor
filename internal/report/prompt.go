package report

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"labmonitor/internal/state"
)

type Style struct {
	Personality string
	SnarkLevel  int
}

type PromptBuilder struct {
	style Style
}

func NewPromptBuilder() PromptBuilder {
	return PromptBuilder{}
}

func NewPromptBuilderWithStyle(style Style) PromptBuilder {
	return PromptBuilder{style: style}
}

type promptPayload struct {
	Timestamp        time.Time           `json:"timestamp"`
	Notes            []string            `json:"notes,omitempty"`
	CurrentSummaries []Summary           `json:"current_summaries"`
	PreviousReport   *state.ReportRecord `json:"previous_report,omitempty"`
	TrendMetrics     *TrendMetrics       `json:"trend_metrics,omitempty"`
	Instructions     string              `json:"instructions"`
}

func (p PromptBuilder) Build(summaries []Summary, last *state.ReportRecord, notes []string, trends *TrendMetrics) (string, error) {
	tone := p.toneInstructions()
	emoji := p.emojiGuidance()
	payload := promptPayload{
		Timestamp:        time.Now().UTC(),
		Notes:            notes,
		CurrentSummaries: summaries,
		PreviousReport:   last,
		TrendMetrics:     trends,
		Instructions: strings.Join([]string{
			"You are monitoring temperature and humidity for multiple labs.",
			"Review the JSON summaries and produce a JSON response matching this schema: {\"status\": string, \"summary\": string, \"labs\": [{\"name\": string, \"status\": string, \"details\": string, \"recommendations\": [string]}], \"recommendations\": [string], \"need_context\": {\"labs\": [{\"name\": string, \"window_hours\": number, \"reason\": string}]} }.",
			"Acceptable status values: normal, watch, alert.",
			"If you require more historical context, populate need_context with specific lab names and exact window_hours you require (integer hours), plus a concise reason. Do not include other content when requesting context.",
			"Historical data requests can span arbitrary windows; the system will stitch together multiple API slices (each capped at roughly 8000 samples), so ask only for the smallest window that unblocks your analysis.",
			"If trend_metrics are present, prefer using those multi-window statistics (6h/24h/72h/168h means, 24h deltas, status_counts) to reason about persistence instead of requesting broader context unless a needed window is missing.",
			"Otherwise, provide actionable analysis covering notable trends, spikes, comfort issues, and recommendations. Prefer concise sentences.",
			"When appropriate based on current data and the previous report, briefly acknowledge sustained improvements (praise) or persistent incidents (incident-aware).",
			tone,
			emoji,
			"Always return valid JSON without markdown fences or commentary.",
		}, "\n"),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal prompt: %w", err)
	}
	return string(data), nil
}

// toneInstructions returns guidance based on the PromptBuilder's style.
func (p PromptBuilder) toneInstructions() string {
	// Defaults
	personality := strings.ToLower(strings.TrimSpace(p.style.Personality))
	snark := p.style.SnarkLevel
	if snark < 0 {
		snark = 0
	}
	if snark > 3 {
		snark = 3
	}

	switch personality {
	case "snarky", "humorous", "friendly":
		// Calibrated tone with an emphasis on professionalism.
		base := "Match the requested tone while staying professional and respectful:"
		var flavor string
		switch personality {
		case "friendly":
			flavor = "friendly, upbeat, and encouraging"
		case "humorous":
			flavor = "lightly humorous with tasteful wit"
		default: // snarky
			// Scale snark with level, but insist on respectful phrasing.
			switch snark {
			case 0, 1:
				flavor = "dry and slightly cheeky (no rudeness)"
			case 2:
				flavor = "cheeky with playful sarcasm (remain respectful)"
			default:
				flavor = "spicy but professional; never insulting or demeaning"
			}
		}
		return fmt.Sprintf("%s %s. Keep it concise. Do not be rude. Focus on clarity.", base, flavor)
	default:
		return "Use a neutral, concise, professional tone."
	}
}

// emojiGuidance tailors emoji usage guidance by personality/snark level.
// For humorous/snarky styles, we explicitly allow broader emoji usage beyond just status icons,
// while keeping it tasteful and sparse. For neutral/friendly, keep minimal.
func (p PromptBuilder) emojiGuidance() string {
	personality := strings.ToLower(strings.TrimSpace(p.style.Personality))
	snark := p.style.SnarkLevel
	if snark < 0 {
		snark = 0
	}
	if snark > 3 {
		snark = 3
	}
	switch personality {
	case "humorous", "snarky":
		// Relax constraints so the model can choose fitting emojis (not just status icons)
		// but keep it tasteful and relevant to the content.
		switch snark {
		case 0, 1:
			return "If the tone is playful, you may sprinkle in occasional, relevant emoji (not only status icons) to add charm. Keep usage light and contextual."
		case 2:
			return "You may use relevant, witty emoji where it enhances the humor or emphasis. Keep it tasteful and avoid clutter."
		default:
			return "Feel free to use expressive, relevant emoji to punch up the humor or snark (beyond status icons), but stay professional and do not overdo it."
		}
	case "friendly":
		return "You may include a few gentle, relevant emoji to keep it upbeat. Use sparingly."
	default:
		return "If the tone is neutral, avoid emoji unless they directly aid clarity."
	}
}
