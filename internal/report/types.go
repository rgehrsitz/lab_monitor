package report

type NeedContextRequest struct {
	Labs []ContextLabRequest `json:"labs"`
}

type ContextLabRequest struct {
	Name        string `json:"name"`
	WindowHours int    `json:"window_hours"`
	Reason      string `json:"reason"`
}

type Assessment struct {
	Status          string              `json:"status"`
	Summary         string              `json:"summary"`
	Labs            []LabAssessment     `json:"labs"`
	Recommendations []string            `json:"recommendations"`
	NeedContext     *NeedContextRequest `json:"need_context,omitempty"`
}

// TrendMetrics provides aggregated multi-window trends per lab. Placeholder
// structure; computation populated upstream before prompt building.
type TrendMetrics struct {
	Labs []LabTrend `json:"labs"`
}

type LabTrend struct {
	Name         string         `json:"name"`
	Temp         MetricTrend    `json:"temp"`
	Humidity     MetricTrend    `json:"humidity"`
	StatusCounts map[string]int `json:"status_counts,omitempty"`
}

type MetricTrend struct {
	Mean    WindowedMeans `json:"mean"`
	Delta24 *float64      `json:"delta_24h,omitempty"`
}

type WindowedMeans struct {
	H6   *float64 `json:"6h,omitempty"`
	H24  *float64 `json:"24h,omitempty"`
	H72  *float64 `json:"72h,omitempty"`
	H168 *float64 `json:"168h,omitempty"`
}

type LabAssessment struct {
	Name            string   `json:"name"`
	Status          string   `json:"status"`
	Details         string   `json:"details"`
	Recommendations []string `json:"recommendations"`
}
