package events

import (
	"time"

	"labmonitor/internal/report"
)

type Kind string

const (
	KindRunStarted       Kind = "run_started"
	KindRunCompleted     Kind = "run_completed"
	KindRunFailed        Kind = "run_failed"
	KindFetchStarted     Kind = "fetch_started"
	KindFetchCompleted   Kind = "fetch_completed"
	KindFetchSplit       Kind = "fetch_split"
	KindContextRequested Kind = "context_requested"
	KindContextProvided  Kind = "context_provided"
	KindAssessmentReady  Kind = "assessment_ready"
	KindEmailAttempt     Kind = "email_attempt"
	KindEmailSent        Kind = "email_sent"
	KindEmailSkipped     Kind = "email_skipped"
	KindStateSaved       Kind = "state_saved"
)

type Event struct {
	Time        time.Time
	Kind        Kind
	Message     string
	LabName     string
	ChannelID   int
	WindowHours int
	Requests    int
	Samples     int
	Summary     *report.Summary
	Assessment  *report.Assessment
	Error       string
}
