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

type LabAssessment struct {
	Name            string   `json:"name"`
	Status          string   `json:"status"`
	Details         string   `json:"details"`
	Recommendations []string `json:"recommendations"`
}
