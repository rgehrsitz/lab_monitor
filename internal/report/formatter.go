package report

import (
	"bytes"
	"fmt"
	"html"
	"strings"
)

// RenderOptions tunes the output style in text/HTML formats.
type RenderOptions struct {
	UseIcons bool
	UseColor bool // Only affects HTML
}

func statusIcon(status string) string {
	switch strings.ToLower(status) {
	case "alert":
		return "🚨"
	case "watch":
		return "⚠️"
	default:
		return "✅"
	}
}

func statusColor(status string) string {
	switch strings.ToLower(status) {
	case "alert":
		return "#b00020" // red
	case "watch":
		return "#c57f00" // amber
	default:
		return "#1b5e20" // green
	}
}

func FormatText(assessment Assessment, summaries []Summary, opts ...RenderOptions) string {
	var buf bytes.Buffer
	var ro RenderOptions
	if len(opts) > 0 {
		ro = opts[0]
	}
	icon := ""
	if ro.UseIcons {
		icon = statusIcon(assessment.Status) + " "
	}
	buf.WriteString(fmt.Sprintf("%sStatus: %s\n", icon, strings.ToUpper(assessment.Status)))
	if assessment.Summary != "" {
		buf.WriteString(fmt.Sprintf("Summary: %s\n\n", assessment.Summary))
	}
	for _, lab := range assessment.Labs {
		ic := ""
		if ro.UseIcons {
			ic = statusIcon(lab.Status) + " "
		}
		buf.WriteString(fmt.Sprintf("%sLab %s [%s]\n", ic, lab.Name, strings.ToUpper(lab.Status)))
		if lab.Details != "" {
			buf.WriteString(fmt.Sprintf("  Details: %s\n", lab.Details))
		}
		if len(lab.Recommendations) > 0 {
			buf.WriteString("  Recommendations:\n")
			for _, rec := range lab.Recommendations {
				buf.WriteString(fmt.Sprintf("    - %s\n", rec))
			}
		}
		buf.WriteString("\n")
	}
	if len(assessment.Recommendations) > 0 {
		buf.WriteString("Overall Recommendations:\n")
		for _, rec := range assessment.Recommendations {
			buf.WriteString(fmt.Sprintf("  - %s\n", rec))
		}
		buf.WriteString("\n")
	}
	if len(summaries) > 0 {
		buf.WriteString("Data Windows:\n")
		for _, s := range summaries {
			buf.WriteString(fmt.Sprintf("  %s (%s): samples=%d temp=%.1f°F avg %.1f°F range %.1f-%.1f°F Δ%.1f°F humidity=%.1f%% avg %.1f%% range %.1f-%.1f%% Δ%.1f%%\n",
				s.LabName,
				s.Context,
				s.Samples,
				s.LatestTemperature,
				s.TemperatureStats.Mean,
				s.TemperatureStats.Min,
				s.TemperatureStats.Max,
				s.TemperatureStats.Delta,
				s.LatestHumidity,
				s.HumidityStats.Mean,
				s.HumidityStats.Min,
				s.HumidityStats.Max,
				s.HumidityStats.Delta,
			))
		}
	}
	return buf.String()
}

func FormatHTML(assessment Assessment, summaries []Summary, opts ...RenderOptions) string {
	var ro RenderOptions
	if len(opts) > 0 {
		ro = opts[0]
	}
	var buf bytes.Buffer
	buf.WriteString("<html><body>")
	overallIcon := ""
	if ro.UseIcons {
		overallIcon = statusIcon(assessment.Status) + " "
	}
	style := ""
	if ro.UseColor {
		style = fmt.Sprintf(" style=\"color:%s\"", statusColor(assessment.Status))
	}
	buf.WriteString(fmt.Sprintf("<h2%s>%sStatus: %s</h2>", style, overallIcon, html.EscapeString(strings.ToUpper(assessment.Status))))
	if assessment.Summary != "" {
		buf.WriteString(fmt.Sprintf("<p>%s</p>", html.EscapeString(assessment.Summary)))
	}
	for _, lab := range assessment.Labs {
		buf.WriteString("<section>")
		labIcon := ""
		if ro.UseIcons {
			labIcon = statusIcon(lab.Status) + " "
		}
		labStyle := ""
		if ro.UseColor {
			labStyle = fmt.Sprintf(" style=\"color:%s\"", statusColor(lab.Status))
		}
		buf.WriteString(fmt.Sprintf("<h3%s>%sLab %s [%s]</h3>", labStyle, labIcon, html.EscapeString(lab.Name), html.EscapeString(strings.ToUpper(lab.Status))))
		if lab.Details != "" {
			buf.WriteString(fmt.Sprintf("<p>%s</p>", html.EscapeString(lab.Details)))
		}
		if len(lab.Recommendations) > 0 {
			buf.WriteString("<ul>")
			for _, rec := range lab.Recommendations {
				buf.WriteString(fmt.Sprintf("<li>%s</li>", html.EscapeString(rec)))
			}
			buf.WriteString("</ul>")
		}
		buf.WriteString("</section>")
	}
	if len(assessment.Recommendations) > 0 {
		buf.WriteString("<section><h3>Overall Recommendations</h3><ul>")
		for _, rec := range assessment.Recommendations {
			buf.WriteString(fmt.Sprintf("<li>%s</li>", html.EscapeString(rec)))
		}
		buf.WriteString("</ul></section>")
	}
	if len(summaries) > 0 {
		buf.WriteString("<section><h3>Data Windows</h3><table border=\"1\" cellpadding=\"4\"><tr><th>Lab</th><th>Context</th><th>Samples</th><th>Temp Latest</th><th>Temp Avg</th><th>Temp Min</th><th>Temp Max</th><th>Temp Δ</th><th>Humidity Latest</th><th>Humidity Avg</th><th>Humidity Min</th><th>Humidity Max</th><th>Humidity Δ</th></tr>")
		for _, s := range summaries {
			buf.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%d</td><td>%.1f°F</td><td>%.1f°F</td><td>%.1f°F</td><td>%.1f°F</td><td>%.1f°F</td><td>%.1f%%</td><td>%.1f%%</td><td>%.1f%%</td><td>%.1f%%</td><td>%.1f%%</td></tr>",
				html.EscapeString(s.LabName),
				html.EscapeString(s.Context),
				s.Samples,
				s.LatestTemperature,
				s.TemperatureStats.Mean,
				s.TemperatureStats.Min,
				s.TemperatureStats.Max,
				s.TemperatureStats.Delta,
				s.LatestHumidity,
				s.HumidityStats.Mean,
				s.HumidityStats.Min,
				s.HumidityStats.Max,
				s.HumidityStats.Delta,
			))
		}
		buf.WriteString("</table></section>")
	}
	buf.WriteString("</body></html>")
	return buf.String()
}
