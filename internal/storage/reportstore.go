package storage

import "labmonitor/internal/state"

// ReportStore persists generated report metadata.
type ReportStore interface {
	SaveReport(state.ReportRecord) error
	LoadHistory() ([]state.ReportRecord, error)
	LatestReport() (*state.ReportRecord, error)
}
