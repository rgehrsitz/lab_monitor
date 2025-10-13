package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"labmonitor/internal/config"
	"labmonitor/internal/email"
	"labmonitor/internal/llm"
	"labmonitor/internal/report"
	"labmonitor/internal/state"
	"labmonitor/internal/thingspeak"
)

const baselineWindow = 24 * time.Hour
const maxContextAttempts = 3

type Service struct {
	cfg            config.Config
	thingspeak     *thingspeak.Client
	aggregator     report.Aggregator
	promptBuilder  report.PromptBuilder
	llmClient      *llm.Client
	emailSender    *email.Sender
	store          *state.Store
	channelsByName map[string]config.ChannelConfig
}

func NewService(cfg config.Config, ts *thingspeak.Client, agg report.Aggregator, pb report.PromptBuilder, llmClient *llm.Client, emailSender *email.Sender, store *state.Store) *Service {
	channels := make(map[string]config.ChannelConfig, len(cfg.Channels))
	for _, ch := range cfg.Channels {
		channels[ch.Name] = ch
	}
	return &Service{
		cfg:            cfg,
		thingspeak:     ts,
		aggregator:     agg,
		promptBuilder:  pb,
		llmClient:      llmClient,
		emailSender:    emailSender,
		store:          store,
		channelsByName: channels,
	}
}

func (s *Service) Run(ctx context.Context, now time.Time) error {
	log.Printf("Starting lab monitor run at %s", now.Format(time.RFC3339))
	refTime := now.UTC()
	summaries := make([]report.Summary, 0, len(s.cfg.Channels))

	log.Printf("Fetching data for %d lab(s)...", len(s.cfg.Channels))
	for i, ch := range s.cfg.Channels {
		log.Printf("[%d/%d] Fetching %s (ID: %d)...", i+1, len(s.cfg.Channels), ch.Name, ch.ID)
		feeds, err := s.thingspeak.GetFeeds(ctx, ch.ID, refTime.Add(-baselineWindow), refTime)
		if err != nil {
			return fmt.Errorf("fetch channel %s: %w", ch.Name, err)
		}
		log.Printf("[%d/%d] Retrieved %d data points for %s", i+1, len(s.cfg.Channels), len(feeds), ch.Name)
		summary := s.aggregator.Summarize(feeds, ch.ID, ch.TemperatureField, ch.HumidityField, baselineWindow, refTime, ch.Name, fmt.Sprintf("baseline %dh", int(baselineWindow.Hours())))
		summaries = append(summaries, summary)
	}
	notes := []string{
		fmt.Sprintf("report_generated_utc=%s", refTime.Format(time.RFC3339)),
		fmt.Sprintf("baseline_window_hours=%d", int(baselineWindow.Hours())),
	}

	log.Println("Loading previous report for context...")
	lastReport, err := s.store.LatestReport()
	if err != nil {
		return fmt.Errorf("load last report: %w", err)
	}

	log.Println("Building analysis prompt...")
	prompt, err := s.promptBuilder.Build(summaries, lastReport, notes)
	if err != nil {
		return fmt.Errorf("build prompt: %w", err)
	}

	log.Println("Requesting AI analysis...")
	result, err := s.llmClient.GenerateAssessment(ctx, prompt)
	if err != nil {
		return fmt.Errorf("llm assessment: %w", err)
	}
	log.Println("AI analysis complete")
	allSummaries := append([]report.Summary(nil), summaries...)
	escalations := make(map[string]int)
	attempt := 1
	for attempt < maxContextAttempts && result.Assessment.NeedContext != nil && len(result.Assessment.NeedContext.Labs) > 0 {
		log.Printf("AI requested additional context for %d lab(s), fetching extended data (attempt %d/%d)...", len(result.Assessment.NeedContext.Labs), attempt+1, maxContextAttempts)
		attempt++
		additionalSummaries := make([]report.Summary, 0, len(result.Assessment.NeedContext.Labs))
		for _, req := range result.Assessment.NeedContext.Labs {
			ch, ok := s.channelsByName[req.Name]
			if !ok {
				notes = append(notes, fmt.Sprintf("ignored_need_context_unknown_lab=%s", req.Name))
				continue
			}
			if req.WindowHours <= 0 {
				notes = append(notes, fmt.Sprintf("ignored_need_context_invalid_window lab=%s window=%d", req.Name, req.WindowHours))
				continue
			}
			window := time.Duration(req.WindowHours) * time.Hour
			log.Printf("  Fetching %dh window for %s (reason: %s)", req.WindowHours, req.Name, req.Reason)
			feeds, err := s.thingspeak.GetFeeds(ctx, ch.ID, refTime.Add(-window), refTime)
			if err != nil {
				return fmt.Errorf("fetch extended window %s: %w", ch.Name, err)
			}
			label := fmt.Sprintf("need_context %dh %s", req.WindowHours, req.Reason)
			summary := s.aggregator.Summarize(feeds, ch.ID, ch.TemperatureField, ch.HumidityField, window, refTime, ch.Name, label)
			additionalSummaries = append(additionalSummaries, summary)
			escalations[ch.Name] = req.WindowHours
			notes = append(notes, fmt.Sprintf("provided_context lab=%s window=%dh reason=%s", ch.Name, req.WindowHours, req.Reason))
		}
		if len(additionalSummaries) == 0 {
			break
		}
		allSummaries = append(allSummaries, additionalSummaries...)
		log.Println("Re-analyzing with additional context...")
		prompt, err = s.promptBuilder.Build(allSummaries, lastReport, notes)
		if err != nil {
			return fmt.Errorf("build extended prompt: %w", err)
		}
		result, err = s.llmClient.GenerateAssessment(ctx, prompt)
		if err != nil {
			return fmt.Errorf("llm extended assessment: %w", err)
		}
		log.Println("Extended analysis complete")
	}

	log.Println("Generating report...")
	textBody := report.FormatText(result.Assessment, allSummaries)
	htmlBody := report.FormatHTML(result.Assessment, allSummaries)
	subject := fmt.Sprintf("%s report", now.In(time.Local).Format("2006-01-02 15:04"))

	log.Printf("Sending report to %d recipient(s)...", len(s.cfg.Email.Recipients))
	if err := s.emailSender.Send(ctx, s.cfg.Email.Recipients, subject, textBody, htmlBody); err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	log.Println("Report sent successfully")
	perLab := make([]state.LabReport, 0, len(result.Assessment.Labs))
	for _, lab := range result.Assessment.Labs {
		perLab = append(perLab, state.LabReport{
			LabName:   lab.Name,
			Summary:   lab.Details,
			Status:    lab.Status,
			Timestamp: now,
		})
	}
	record := state.ReportRecord{
		Timestamp:    now,
		Overall:      result.Assessment.Status,
		PerLab:       perLab,
		RawResponse:  result.Raw,
		Escalations:  escalations,
		PromptDigest: hashString(prompt),
	}
	log.Println("Saving report to state store...")
	if err := s.store.SaveReport(record); err != nil {
		return fmt.Errorf("save report: %w", err)
	}
	log.Printf("Run complete - Overall status: %s", record.Overall)
	return nil
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
