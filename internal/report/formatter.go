package report

import (
	"bytes"
	"fmt"
	"html"
	"strings"
)

func FormatText(assessment Assessment, summaries []Summary) string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("Status: %s\n", strings.ToUpper(assessment.Status)))
	if assessment.Summary != "" {
		buf.WriteString(fmt.Sprintf("Summary: %s\n\n", assessment.Summary))
	}
	for _, lab := range assessment.Labs {
		buf.WriteString(fmt.Sprintf("Lab %s [%s]\n", lab.Name, strings.ToUpper(lab.Status)))
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

func FormatHTML(assessment Assessment, summaries []Summary) string {
	var buf bytes.Buffer
	buf.WriteString("<html><body>")
	buf.WriteString(fmt.Sprintf("<h2>Status: %s</h2>", html.EscapeString(strings.ToUpper(assessment.Status))))
	if assessment.Summary != "" {
		buf.WriteString(fmt.Sprintf("<p>%s</p>", html.EscapeString(assessment.Summary)))
	}
	for _, lab := range assessment.Labs {
		buf.WriteString("<section>")
		buf.WriteString(fmt.Sprintf("<h3>Lab %s [%s]</h3>", html.EscapeString(lab.Name), html.EscapeString(strings.ToUpper(lab.Status))))
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
