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
			"CRITICAL DATA PRIORITY: Current summaries represent real-time sensor data and take absolute precedence over historical reports when determining current lab status. If current_summaries show samples > 0 for a lab, that lab is currently operational regardless of previous reports.",
			"⚠️ MANDATORY REQUIREMENT - DATA COVERAGE TRANSPARENCY:",
			"  • ALWAYS calculate: hours_covered = samples ÷ 12 (5-min intervals)",
			"  • IF hours_covered < 20: You MUST state 'Assessment based on limited data covering N hours' AND include 'Full 24-hour trend analysis will be available once sufficient data accumulates.'",
			"  • IF hours_covered >= 20: State 'Assessment based on N hours of data' (no disclaimer needed)",
			"  • EXAMPLE for 114 samples (9.5 hours): 'Lab X shows continuous operation with 114 samples covering approximately 9.5 hours. Assessment based on this limited timeframe shows [temperature/humidity]. Full 24-hour trend analysis will be available once sufficient data accumulates.'",
			"⚠️ CRITICAL: RECOVERY vs CONTINUOUS OPERATION - DO NOT CONFUSE THESE:",
			"  • RECOVERY PATTERN: previous_report shows 'no telemetry/offline/no data' for Lab X → current_summaries shows Lab X has samples > 0. USE: 'Lab X has reconnected', 'since recovery', 'back online'.",
			"  • CONTINUOUS OPERATION PATTERN: previous_report shows Lab Y was operational → current_summaries shows Lab Y has ~22-24 hours of data (264-288 samples).",
			"    ❌ FORBIDDEN PHRASES for continuous labs: 'since recovery', 'resumed', 'restarted', 'getting back to work', 'back online', 'since it got rolling', 'since it kicked off', 'since operations began', 'since operations resumed', 'since restarting'.",
			"    ✅ REQUIRED PHRASES for continuous labs: 'shows continuous operation', 'continues operating normally', 'maintains operation', 'is operating consistently'.",
			"  • Check EACH lab's previous_report individually. Lab A being offline does NOT mean Lab B was offline. NEVER apply recovery language to labs that had continuous operation.",
			"OPERATIONAL STATUS RULES: A lab with samples > 0 in current_summaries is OPERATIONAL. Do not classify it as having 'connectivity issues' or 'device malfunction' based on historical reports. Focus on the actual environmental data quality and readings.",
			"HISTORICAL CONTEXT USAGE: Use previous_report for trend analysis, persistence patterns, and acknowledging improvements/deteriorations, but never let historical 'offline' status override current data availability when assessing present operational status. MORE IMPORTANTLY: Check each lab's previous_report individually to determine if IT was offline - do not assume all labs had the same status.",
			"If you require more historical context, populate need_context with specific lab names and exact window_hours you require (integer hours), plus a concise reason. Do not include other content when requesting context.",
			"Historical data requests can span arbitrary windows; the system will stitch together multiple API slices (each capped at roughly 8000 samples), so ask only for the smallest window that unblocks your analysis.",
			"If trend_metrics are present, prefer using those multi-window statistics (6h/24h/72h/168h means, 24h deltas, status_counts) to reason about persistence instead of requesting broader context unless a needed window is missing.",
			"Notes with the prefix 'trend' summarise significant temperature or humidity movements. You must explicitly discuss each applicable trend in both the overall summary and the relevant lab details (e.g., highlight cooling/heating rates or humidity shifts).",
			"DATA GAP vs CURRENT STATUS: Distinguish between historical data gaps (which are acceptable to mention for context) and current operational status. A lab showing recent samples has recovered from any previous outages - assess its current environmental conditions normally.",
			"USER-CENTRIC COMMUNICATION: Think about what information users need to make informed decisions. If data coverage is limited, explain the implications. If a sensor just recovered, mention when it came back online (if inferable from timestamps). Help users understand confidence levels in your assessment.",
			"Otherwise, provide actionable analysis covering notable trends, spikes, comfort issues, and recommendations. Prefer concise sentences.",
			"When appropriate based on current data and the previous report, briefly acknowledge sustained improvements (praise) or persistent incidents (incident-aware). For recovered sensors, note the recovery, explain data coverage, and assess current conditions based on available data.",
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
