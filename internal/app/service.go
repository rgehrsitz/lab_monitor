package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"labmonitor/internal/config"
	"labmonitor/internal/email"
	"labmonitor/internal/events"
	"labmonitor/internal/llm"
	"labmonitor/internal/report"
	"labmonitor/internal/state"
	"labmonitor/internal/storage"
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
	store          storage.ReportStore
	channelsByName map[string]config.ChannelConfig
	events         chan<- events.Event
}

// computeTrendMetrics fetches multi-window summaries per lab to derive mean stats and status counts from history.
// It reuses the ThingSpeak client; for each channel we fetch four windows (6h,24h,72h,168h) unless baseline already covers one.
func (s *Service) computeTrendMetrics(ctx context.Context, ref time.Time) *report.TrendMetrics {
	windows := []time.Duration{6 * time.Hour, 24 * time.Hour, 72 * time.Hour, 168 * time.Hour}
	// Load history for status counts
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
	trendLabs := make([]report.LabTrend, 0, len(s.cfg.Channels))
	for _, ch := range s.cfg.Channels {
		meansTemp := report.WindowedMeans{}
		meansHum := report.WindowedMeans{}
		var meanMap = map[int]report.Summary{}
		for _, w := range windows {
			feeds, _, err := s.thingspeak.GetFeeds(ctx, ch.ID, ref.Add(-w), ref)
			if err != nil {
				continue
			}
			sum := s.aggregator.Summarize(feeds, ch.ID, ch.TemperatureField, ch.HumidityField, w, ref, ch.Name, fmt.Sprintf("trend %dh", int(w.Hours())))
			meanMap[int(w.Hours())] = sum
		}
		assign := func(ptr **float64, v float64) {
			vv := v
			*ptr = &vv
		}
		if s, ok := meanMap[6]; ok && s.Samples > 0 {
			assign(&meansTemp.H6, s.TemperatureStats.Mean)
			assign(&meansHum.H6, s.HumidityStats.Mean)
		}
		if s, ok := meanMap[24]; ok && s.Samples > 0 {
			assign(&meansTemp.H24, s.TemperatureStats.Mean)
			assign(&meansHum.H24, s.HumidityStats.Mean)
		}
		if s, ok := meanMap[72]; ok && s.Samples > 0 {
			assign(&meansTemp.H72, s.TemperatureStats.Mean)
			assign(&meansHum.H72, s.HumidityStats.Mean)
		}
		if s, ok := meanMap[168]; ok && s.Samples > 0 {
			assign(&meansTemp.H168, s.TemperatureStats.Mean)
			assign(&meansHum.H168, s.HumidityStats.Mean)
		}
		var delta24temp *float64
		var delta24hum *float64
		// Delta24 = difference between last value of current 24h window and mean of prior 24h block (24-48h ago). Fetch prior window if possible.
		feedsPrev, _, err := s.thingspeak.GetFeeds(ctx, ch.ID, ref.Add(-48*time.Hour), ref.Add(-24*time.Hour))
		if err == nil && len(feedsPrev) > 0 {
			prevSum := s.aggregator.Summarize(feedsPrev, ch.ID, ch.TemperatureField, ch.HumidityField, 24*time.Hour, ref.Add(-24*time.Hour), ch.Name, "trend prev24h")
			if cur, ok := meanMap[24]; ok && cur.Samples > 0 && prevSum.Samples > 0 {
				// Use difference of means
				dt := cur.TemperatureStats.Mean - prevSum.TemperatureStats.Mean
				dh := cur.HumidityStats.Mean - prevSum.HumidityStats.Mean
				dtCopy := dt
				dhCopy := dh
				delta24temp = &dtCopy
				delta24hum = &dhCopy
			}
		}
		labTrend := report.LabTrend{Name: ch.Name, Temp: report.MetricTrend{Mean: meansTemp, Delta24: delta24temp}, Humidity: report.MetricTrend{Mean: meansHum, Delta24: delta24hum}, StatusCounts: statusCounts[ch.Name]}
		trendLabs = append(trendLabs, labTrend)
	}
	// sort by name for deterministic ordering
	for i := 0; i < len(trendLabs)-1; i++ {
		for j := i + 1; j < len(trendLabs); j++ {
			if trendLabs[j].Name < trendLabs[i].Name {
				trendLabs[i], trendLabs[j] = trendLabs[j], trendLabs[i]
			}
		}
	}
	return &report.TrendMetrics{Labs: trendLabs}
}

func NewService(cfg config.Config, ts *thingspeak.Client, agg report.Aggregator, pb report.PromptBuilder, llmClient *llm.Client, emailSender *email.Sender, store storage.ReportStore, events chan<- events.Event) *Service {
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
	trends := s.computeTrendMetrics(ctx, refTime)

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

	// Configure LLM client runtime parameters (idempotent each run)
	s.llmClient.Configure(
		time.Duration(s.cfg.OpenAI.InitialTimeoutSeconds)*time.Second,
		time.Duration(s.cfg.OpenAI.ExtensionTimeoutSeconds)*time.Second,
		time.Duration(s.cfg.OpenAI.RequestTimeoutSeconds)*time.Second,
		time.Duration(s.cfg.OpenAI.ToneTimeoutSeconds)*time.Second,
		s.cfg.OpenAI.MaxRetries,
		time.Duration(s.cfg.OpenAI.RetryBackoffMs)*time.Millisecond,
	)
	// Attach event emitter for observability
	s.llmClient.SetEventEmitter(func(meta map[string]any) {
		// Convert to event
		s.emit(events.KindLLMCall, func(e *events.Event) {
			// Pack key details into Message
			if m, ok := meta["kind"].(string); ok {
				status := "ok"
				if succ, ok2 := meta["success"].(bool); ok2 && !succ {
					status = "fail"
				}
				lat := meta["latency_ms"]
				e.Message = fmt.Sprintf("%s %s latency_ms=%v attempt=%v extended=%v", m, status, lat, meta["attempt"], meta["extended"])
			}
			if errStr, ok := meta["error"].(string); ok {
				e.Error = errStr
			}
		})
	})
	log.Println("Requesting AI analysis (initial)...")
	result, err := s.llmClient.GenerateAssessment(ctx, prompt, false)
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
		// Coalesce by lab selecting the largest requested window per lab this round.
		type reqInfo struct {
			window time.Duration
			reason string
		}
		labRequests := map[string]reqInfo{}
		for _, r := range result.Assessment.NeedContext.Labs {
			w := time.Duration(r.WindowHours) * time.Hour
			if w <= 0 || w > 7*24*time.Hour {
				w = baselineWindow
			}
			if existing, ok := labRequests[r.Name]; !ok || w > existing.window {
				labRequests[r.Name] = reqInfo{window: w, reason: r.Reason}
			}
		}
		// Ensure the model isn't repeating exactly same coalesced pattern.
		rawReq, _ := json.Marshal(labRequests)
		reqHash := hashString(string(rawReq))
		if reqHash == prevRequestsHash {
			log.Println("Detected repeated context request pattern after coalescing; halting additional fetches.")
			break
		}
		prevRequestsHash = reqHash
		additionalSummaries := make([]report.Summary, 0, len(labRequests))
		s.emit(events.KindContextRequested, func(e *events.Event) { e.Message = fmt.Sprintf("%d request(s) coalesced", len(labRequests)) })
		const maxCumulativeHours = 168 // 7d cap
		for name, info := range labRequests {
			ch, ok := s.channelsByName[name]
			if !ok {
				continue
			}
			// Cap escalation per lab.
			already := escalations[name]
			addHours := int(info.window.Hours())
			if already+addHours > maxCumulativeHours {
				addHours = maxCumulativeHours - already
				if addHours <= 0 {
					continue
				}
				info.window = time.Duration(addHours) * time.Hour
			}
			feeds, stats, err := s.thingspeak.GetFeeds(ctx, ch.ID, refTime.Add(-info.window), refTime)
			if err != nil {
				log.Printf("context fetch failed for %s: %v", ch.Name, err)
				continue
			}
			summary := s.aggregator.Summarize(feeds, ch.ID, ch.TemperatureField, ch.HumidityField, info.window, refTime, ch.Name, fmt.Sprintf("context %dh", int(info.window.Hours())))
			additionalSummaries = append(additionalSummaries, summary)
			s.emit(events.KindContextProvided, func(e *events.Event) {
				e.LabName = ch.Name
				e.ChannelID = ch.ID
				e.WindowHours = int(info.window.Hours())
				e.Requests = stats.Requests
				e.Samples = len(feeds)
				e.Message = info.reason
				e.Summary = &summary
			})
			escalations[ch.Name] += int(info.window.Hours())
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
		result, err = s.llmClient.GenerateAssessment(ctx, prompt, true)
		if err != nil {
			log.Printf("Extended assessment failed (graceful halt): %v", err)
			// Graceful: keep last successful result and stop loop
			break
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
				log.Printf("tone transform failed for profile %s (falling back to canonical): %v", profile.Name, err)
			} else {
				var ta report.Assessment
				if err := json.Unmarshal([]byte(tj), &ta); err != nil {
					log.Printf("parse transformed assessment failed for profile %s (fallback): %v", profile.Name, err)
				} else {
					transformed = ta
				}
			}
		}
		useIcons := ro.UseIcons
		useColor := ro.UseColor
		if profile.UseIcons != nil {
			useIcons = *profile.UseIcons
		}
		if profile.UseColor != nil {
			useColor = *profile.UseColor
		}
		// For humorous/snarky profiles, if not explicitly set, prefer not forcing fixed status icons
		// so the model's chosen emoji can be used freely in text.
		if profile.UseIcons == nil {
			p := strings.ToLower(strings.TrimSpace(profile.Personality))
			if p == "humorous" || p == "snarky" {
				useIcons = false
			}
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
