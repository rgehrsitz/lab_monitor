package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"labmonitor/internal/config"
	"labmonitor/internal/email"
	"labmonitor/internal/events"
	"labmonitor/internal/llm"
	"labmonitor/internal/report"
	"labmonitor/internal/state"
	"labmonitor/internal/thingspeak"
)

const baselineWindow = 24 * time.Hour

// defaultMaxContextAttempts is used when config doesn't specify a value.
const defaultMaxContextAttempts = 3

type Service struct {
	cfg            config.Config
	thingspeak     *thingspeak.Client
	aggregator     report.Aggregator
	promptBuilder  report.PromptBuilder
	llmClient      *llm.Client
	emailSender    *email.Sender
	store          *state.Store
	channelsByName map[string]config.ChannelConfig
	events         chan<- events.Event
}

func NewService(cfg config.Config, ts *thingspeak.Client, agg report.Aggregator, pb report.PromptBuilder, llmClient *llm.Client, emailSender *email.Sender, store *state.Store, events chan<- events.Event) *Service {
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
		events:         events,
	}
}

func (s *Service) Run(ctx context.Context, now time.Time) error {
	log.Printf("Starting lab monitor run at %s", now.Format(time.RFC3339))
	s.emit(events.KindRunStarted, func(e *events.Event) {
		e.Message = now.Format(time.RFC3339)
	})
	refTime := now.UTC()
	summaries := make([]report.Summary, 0, len(s.cfg.Channels))

	log.Printf("Fetching data for %d lab(s)...", len(s.cfg.Channels))
	for i, ch := range s.cfg.Channels {
		log.Printf("[%d/%d] Fetching %s (ID: %d)...", i+1, len(s.cfg.Channels), ch.Name, ch.ID)
		s.emit(events.KindFetchStarted, func(e *events.Event) {
			e.LabName = ch.Name
			e.ChannelID = ch.ID
			e.WindowHours = int(baselineWindow.Hours())
		})
		feeds, stats, err := s.thingspeak.GetFeeds(ctx, ch.ID, refTime.Add(-baselineWindow), refTime)
		if err != nil {
			runErr := fmt.Errorf("fetch channel %s: %w", ch.Name, err)
			s.emit(events.KindRunFailed, func(e *events.Event) {
				e.Error = runErr.Error()
			})
			return runErr
		}
		log.Printf("[%d/%d] Retrieved %d data points for %s", i+1, len(s.cfg.Channels), len(feeds), ch.Name)
		summary := s.aggregator.Summarize(feeds, ch.ID, ch.TemperatureField, ch.HumidityField, baselineWindow, refTime, ch.Name, fmt.Sprintf("baseline %dh", int(baselineWindow.Hours())))
		s.emit(events.KindFetchCompleted, func(e *events.Event) {
			e.LabName = ch.Name
			e.ChannelID = ch.ID
			e.WindowHours = int(baselineWindow.Hours())
			e.Samples = len(feeds)
			e.Requests = stats.Requests
			if stats.Splits > 0 {
				e.Message = fmt.Sprintf("split into %d requests", stats.Requests)
			}
			e.Summary = &summary
		})
		summaries = append(summaries, summary)
	}
	// Informative notes for the model that don't change semantics but can
	// help it self-limit context requests.
	notes := []string{
		fmt.Sprintf("report_generated_utc=%s", refTime.Format(time.RFC3339)),
		fmt.Sprintf("baseline_window_hours=%d", int(baselineWindow.Hours())),
	}

	log.Println("Loading previous report for context...")
	lastReport, err := s.store.LatestReport()
	if err != nil {
		return fmt.Errorf("load last report: %w", err)
	}
	// Load full history for placeholder trend metrics (status counts only for now)
	history, _ := s.store.LoadHistory()
	statusCounts := make(map[string]map[string]int)
	for _, rec := range history {
		for _, lab := range rec.PerLab {
			if statusCounts[lab.LabName] == nil {
				statusCounts[lab.LabName] = map[string]int{}
			}
			statusCounts[lab.LabName][lab.Status]++
		}
	}
	var trendLabs []report.LabTrend
	for name, counts := range statusCounts {
		trendLabs = append(trendLabs, report.LabTrend{Name: name, StatusCounts: counts, Temp: report.MetricTrend{Mean: report.WindowedMeans{}}, Humidity: report.MetricTrend{Mean: report.WindowedMeans{}}})
	}
	trends := &report.TrendMetrics{Labs: trendLabs}

	log.Println("Building analysis prompt...")
	prompt, err := s.promptBuilder.Build(summaries, lastReport, notes, trends)
	if err != nil {
		runErr := fmt.Errorf("build prompt: %w", err)
		s.emit(events.KindRunFailed, func(e *events.Event) {
			e.Error = runErr.Error()
		})
		return runErr
	}

	// Determine context attempt budget for this run
	maxAttempts := s.cfg.OpenAI.MaxContextAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxContextAttempts
	}
	notes = append(notes,
		fmt.Sprintf("max_context_attempts=%d", maxAttempts),
		"guidance: request the smallest additional window that unblocks your analysis; group labs into one request when they share the same reason/window.",
	)

	log.Println("Requesting AI analysis...")
	result, err := s.llmClient.GenerateAssessment(ctx, prompt)
	if err != nil {
		runErr := fmt.Errorf("llm assessment: %w", err)
		s.emit(events.KindRunFailed, func(e *events.Event) {
			e.Error = runErr.Error()
		})
		return runErr
	}
	// Prepare for possible context expansion loop.
	allSummaries := append([]report.Summary(nil), summaries...)
	escalations := map[string]int{}
	attempt := 0
	prevRequestsHash := ""
	for attempt < maxAttempts && result.Assessment.NeedContext != nil && len(result.Assessment.NeedContext.Labs) > 0 {
		log.Printf("AI requested additional context for %d lab(s), fetching extended data (attempt %d/%d)...", len(result.Assessment.NeedContext.Labs), attempt+1, maxAttempts)
		attempt++
		// Ensure the model isn't repeating the same request pattern to avoid infinite loops.
		rawReq, _ := json.Marshal(result.Assessment.NeedContext.Labs)
		reqHash := hashString(string(rawReq))
		if reqHash == prevRequestsHash {
			log.Println("Detected repeated context request pattern; halting additional fetches.")
			break
		}
		prevRequestsHash = reqHash
		additionalSummaries := make([]report.Summary, 0, len(result.Assessment.NeedContext.Labs))
		s.emit(events.KindContextRequested, func(e *events.Event) {
			e.Message = fmt.Sprintf("%d request(s)", len(result.Assessment.NeedContext.Labs))
		})
		for _, req := range result.Assessment.NeedContext.Labs {
			ch, ok := s.channelsByName[req.Name]
			if !ok {
				continue
			}
			window := time.Duration(req.WindowHours) * time.Hour
			if window <= 0 || window > 7*24*time.Hour { // guardrail
				window = baselineWindow
			}
			feeds, stats, err := s.thingspeak.GetFeeds(ctx, ch.ID, refTime.Add(-window), refTime)
			if err != nil {
				log.Printf("context fetch failed for %s: %v", ch.Name, err)
				continue
			}
			summary := s.aggregator.Summarize(feeds, ch.ID, ch.TemperatureField, ch.HumidityField, window, refTime, ch.Name, fmt.Sprintf("context %dh", int(window.Hours())))
			additionalSummaries = append(additionalSummaries, summary)
			s.emit(events.KindContextProvided, func(e *events.Event) {
				e.LabName = ch.Name
				e.ChannelID = ch.ID
				e.WindowHours = req.WindowHours
				e.Requests = stats.Requests
				e.Samples = len(feeds)
				e.Message = req.Reason
				e.Summary = &summary
			})
			escalations[ch.Name] += req.WindowHours
		}
		if len(additionalSummaries) == 0 {
			break
		}
		allSummaries = append(allSummaries, additionalSummaries...)
		log.Println("Re-analyzing with additional context...")
		prompt, err = s.promptBuilder.Build(allSummaries, lastReport, notes, trends)
		if err != nil {
			runErr := fmt.Errorf("build extended prompt: %w", err)
			s.emit(events.KindRunFailed, func(e *events.Event) { e.Error = runErr.Error() })
			return runErr
		}
		result, err = s.llmClient.GenerateAssessment(ctx, prompt)
		if err != nil {
			runErr := fmt.Errorf("llm extended assessment: %w", err)
			s.emit(events.KindRunFailed, func(e *events.Event) { e.Error = runErr.Error() })
			return runErr
		}
		log.Println("Extended analysis complete")
		s.emit(events.KindAssessmentReady, func(e *events.Event) { e.Assessment = &result.Assessment })
	}

	log.Println("Generating report...")
	if len(s.cfg.Email.Profiles) == 0 {
		return fmt.Errorf("no email profiles configured (hard error)")
	}
	ro := report.RenderOptions{UseIcons: s.cfg.Style.UseIcons, UseColor: s.cfg.Style.UseColor}
	subject := fmt.Sprintf("%s report", now.In(time.Local).Format("2006-01-02 15:04"))
	canonicalJSON := result.Raw
	canonicalAssessment := result.Assessment
	log.Printf("Sending report via %d profile(s)...", len(s.cfg.Email.Profiles))
	for _, profile := range s.cfg.Email.Profiles {
		transformed := canonicalAssessment
		if profile.Personality != "" && profile.Personality != s.cfg.Style.Personality {
			tj, err := s.llmClient.TransformAssessmentTone(ctx, canonicalJSON, profile.Personality, profile.SnarkLevel)
			if err != nil {
				runErr := fmt.Errorf("tone transform profile %s: %w", profile.Name, err)
				s.emit(events.KindRunFailed, func(e *events.Event) { e.Error = runErr.Error() })
				return runErr
			}
			var ta report.Assessment
			if err := json.Unmarshal([]byte(tj), &ta); err != nil {
				runErr := fmt.Errorf("parse transformed assessment profile %s: %w", profile.Name, err)
				s.emit(events.KindRunFailed, func(e *events.Event) { e.Error = runErr.Error() })
				return runErr
			}
			transformed = ta
		}
		useIcons := ro.UseIcons
		useColor := ro.UseColor
		if profile.UseIcons != nil {
			useIcons = *profile.UseIcons
		}
		if profile.UseColor != nil {
			useColor = *profile.UseColor
		}
		pro := report.RenderOptions{UseIcons: useIcons, UseColor: useColor}
		textBody := report.FormatText(transformed, allSummaries, pro)
		htmlBody := report.FormatHTML(transformed, allSummaries, pro)
		s.emit(events.KindEmailAttempt, func(e *events.Event) { e.Message = subject })
		if err := s.emailSender.Send(ctx, profile.Recipients, subject, textBody, htmlBody); err != nil {
			runErr := fmt.Errorf("send email profile %s: %w", profile.Name, err)
			s.emit(events.KindRunFailed, func(e *events.Event) { e.Error = runErr.Error() })
			return runErr
		}
	}
	log.Println("Report sent successfully")
	s.emit(events.KindEmailSent, func(e *events.Event) { e.Message = subject })

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
		runErr := fmt.Errorf("save report: %w", err)
		s.emit(events.KindRunFailed, func(e *events.Event) {
			e.Error = runErr.Error()
		})
		return runErr
	}
	s.emit(events.KindStateSaved, func(e *events.Event) {
		e.Message = record.PromptDigest
	})
	log.Printf("Run complete - Overall status: %s", record.Overall)
	s.emit(events.KindRunCompleted, func(e *events.Event) {
		e.Assessment = &result.Assessment
	})
	return nil
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Service) emit(kind events.Kind, enrich func(*events.Event)) {
	if s.events == nil {
		return
	}
	ev := events.Event{
		Time: time.Now(),
		Kind: kind,
	}
	if enrich != nil {
		enrich(&ev)
	}
	select {
	case s.events <- ev:
	default:
	}
}
