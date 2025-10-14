package dashboard

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"

	"labmonitor/internal/config"
	"labmonitor/internal/events"
	"labmonitor/internal/report"
)

const (
	maxLogEntries    = 12
	cardContentWidth = 75
	logContentWidth  = 75
)

type Model struct {
	events   <-chan events.Event
	schedule []string
	dryRun   bool
	loc      *time.Location

	runActive       bool
	runStartedAt    time.Time
	lastRunFinished time.Time
	overallStatus   string
	lastError       string
	lastRunDuration time.Duration

	labs map[string]*labState
	log  []string
}

type labState struct {
	channelID int
	summary   *report.Summary
	status    string
	details   string
	requests  int
	updated   time.Time
}

type eventMsg struct {
	event events.Event
}

type streamClosedMsg struct{}

func NewModel(cfg config.Config, dryRun bool, events <-chan events.Event, loc *time.Location) Model {
	labs := make(map[string]*labState, len(cfg.Channels))
	for _, ch := range cfg.Channels {
		labs[ch.Name] = &labState{channelID: ch.ID}
	}
	schedule := cfg.Schedule.Times
	if loc == nil {
		loc = time.Local
	}
	return Model{
		events:   events,
		schedule: schedule,
		dryRun:   dryRun,
		loc:      loc,
		labs:     labs,
		log:      make([]string, 0, maxLogEntries),
	}
}

func (m Model) Init() tea.Cmd {
	if m.events == nil {
		return nil
	}
	return waitForEvent(m.events)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case eventMsg:
		m.handleEvent(msg.event)
		if m.events == nil {
			return m, nil
		}
		return m, waitForEvent(m.events)
	case streamClosedMsg:
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) View() string {
	now := time.Now().In(m.loc)
	nextRun := m.nextScheduledRun(now)
	status := "IDLE"
	if m.runActive {
		status = "RUNNING"
	}

	var meta []string
	meta = append(meta, metaLabelStyle.Render("Status")+" "+statusStyle(status).Render(status))
	if !m.runStartedAt.IsZero() {
		meta = append(meta, metaLabelStyle.Render("Started")+" "+m.runStartedAt.In(m.loc).Format(time.Kitchen))
	}
	if !m.lastRunFinished.IsZero() {
		finished := m.lastRunFinished.In(m.loc).Format(time.Kitchen)
		if m.lastRunDuration > 0 {
			finished = fmt.Sprintf("%s (%s)", finished, m.lastRunDuration.Round(time.Second))
		}
		meta = append(meta, metaLabelStyle.Render("Last Run")+" "+finished)
	}
	if nextRun.IsZero() {
		meta = append(meta, metaLabelStyle.Render("Next Run")+" awaiting schedule")
	} else {
		remaining := nextRun.Sub(now).Round(time.Second)
		meta = append(meta, metaLabelStyle.Render("Next Run")+" "+nextRun.Format(time.Kitchen)+fmt.Sprintf(" (%s)", remaining))
	}
	if m.overallStatus != "" {
		meta = append(meta, metaLabelStyle.Render("Overall")+" "+statusStyle(m.overallStatus).Render(strings.ToUpper(m.overallStatus)))
	}
	if m.dryRun {
		meta = append(meta, metaLabelStyle.Render("Mode")+" DRY-RUN")
	}
	if len(m.schedule) > 0 {
		meta = append(meta, metaLabelStyle.Render("Schedule")+" "+strings.Join(m.schedule, ", "))
	}
	if m.lastError != "" {
		meta = append(meta, metaLabelStyle.Render("Error")+" "+errorStyle.Render(m.lastError))
	}
	header := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Lab Monitor Dashboard"),
		strings.Join(meta, "\n"),
	)

	labsView := m.renderLabs()
	logView := m.renderLog()

	body := lipgloss.JoinVertical(lipgloss.Left, header, labsView, logView, footerStyle.Render("Press q to exit dashboard"))
	return lipgloss.NewStyle().Padding(1, 2).Render(body)
}

func waitForEvent(ch <-chan events.Event) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
			return streamClosedMsg{}
		}
		ev, ok := <-ch
		if !ok {
			return streamClosedMsg{}
		}
		return eventMsg{event: ev}
	}
}

func (m *Model) handleEvent(ev events.Event) {
	switch ev.Kind {
	case events.KindRunStarted:
		m.runActive = true
		m.runStartedAt = ev.Time
		m.addLog(ev, "Run started")
	case events.KindRunCompleted:
		m.runActive = false
		m.lastRunFinished = ev.Time
		if !m.runStartedAt.IsZero() {
			m.lastRunDuration = ev.Time.Sub(m.runStartedAt)
		}
		if ev.Assessment != nil {
			m.overallStatus = ev.Assessment.Status
			m.refreshLabStatuses(ev.Assessment, ev.Time)
		}
		m.addLog(ev, "Run completed")
	case events.KindRunFailed:
		m.runActive = false
		m.lastError = ev.Error
		if ev.Error != "" {
			m.addLog(ev, fmt.Sprintf("Run failed: %s", ev.Error))
		}
	case events.KindFetchStarted:
		if ev.LabName != "" {
			m.addLog(ev, fmt.Sprintf("Fetching %s (%dh)", ev.LabName, ev.WindowHours))
		}
	case events.KindFetchCompleted, events.KindContextProvided:
		state := m.ensureLab(ev.LabName, ev.ChannelID)
		state.requests = ev.Requests
		state.updated = ev.Time
		if ev.Summary != nil {
			cloned := *ev.Summary
			state.summary = &cloned
		}
		if ev.Message != "" {
			m.addLog(ev, fmt.Sprintf("%s: %s", ev.LabName, ev.Message))
		} else {
			m.addLog(ev, fmt.Sprintf("Fetched %s (%d samples in %d request(s))", ev.LabName, ev.Samples, ev.Requests))
		}
	case events.KindContextRequested:
		m.addLog(ev, "LLM requested additional context")
	case events.KindAssessmentReady:
		if ev.Assessment != nil {
			m.overallStatus = ev.Assessment.Status
			m.refreshLabStatuses(ev.Assessment, ev.Time)
			m.addLog(ev, "Assessment updated")
		}
	case events.KindEmailAttempt:
		m.addLog(ev, fmt.Sprintf("Email preparing: %s", ev.Message))
	case events.KindEmailSent:
		m.addLog(ev, "Email sent")
	case events.KindStateSaved:
		m.addLog(ev, "State updated")
	}
}

func (m *Model) addLog(ev events.Event, message string) {
	if message == "" {
		return
	}
	line := fmt.Sprintf("[%s] %s", ev.Time.Local().Format(time.Kitchen), message)
	m.log = append([]string{line}, m.log...)
	if len(m.log) > maxLogEntries {
		m.log = m.log[:maxLogEntries]
	}
}

func (m *Model) ensureLab(name string, channelID int) *labState {
	if name == "" {
		name = "unknown"
	}
	lab, ok := m.labs[name]
	if !ok {
		lab = &labState{channelID: channelID}
		m.labs[name] = lab
	}
	if channelID != 0 {
		lab.channelID = channelID
	}
	return lab
}

func (m *Model) refreshLabStatuses(assessment *report.Assessment, ts time.Time) {
	if assessment == nil {
		return
	}
	for _, lab := range assessment.Labs {
		state := m.ensureLab(lab.Name, 0)
		state.status = lab.Status
		state.details = lab.Details
		state.updated = ts
	}
}

func (m Model) renderLabs() string {
	if len(m.labs) == 0 {
		return ""
	}
	names := make([]string, 0, len(m.labs))
	for name := range m.labs {
		names = append(names, name)
	}
	sort.Strings(names)

	cards := make([]string, 0, len(names))
	for _, name := range names {
		lab := m.labs[name]
		status := lab.status
		if status == "" {
			status = "unknown"
		}
		statusBlock := statusStyle(status).Bold(true).Render(strings.ToUpper(status))
		lines := []string{labTitleStyle.Render(fmt.Sprintf("%s  #%d", name, lab.channelID)), statusBlock}
		if lab.summary != nil {
			sum := lab.summary
			lines = appendWrappedIndent(lines, infoStyle.Render(fmt.Sprintf("Window %s • Samples %d • Requests %d", sum.WindowLabel, sum.Samples, lab.requests)), cardContentWidth, "  ")
			lines = appendWrappedIndent(lines, infoStyle.Render(fmt.Sprintf("Temp %.1f°F avg %.1f°F range %.1f-%.1f°F", sum.LatestTemperature, sum.TemperatureStats.Mean, sum.TemperatureStats.Min, sum.TemperatureStats.Max)), cardContentWidth, "  ")
			lines = appendWrappedIndent(lines, infoStyle.Render(fmt.Sprintf("Humidity %.1f%% avg %.1f%% range %.1f-%.1f%%", sum.LatestHumidity, sum.HumidityStats.Mean, sum.HumidityStats.Min, sum.HumidityStats.Max)), cardContentWidth, "  ")
		} else {
			lines = appendWrappedIndent(lines, infoStyle.Render("No data yet"), cardContentWidth, "  ")
		}
		if lab.details != "" {
			lines = appendWrappedIndent(lines, detailStyle.Render(lab.details), cardContentWidth, "  ")
		}
		if !lab.updated.IsZero() {
			lines = appendWrappedIndent(lines, timestampStyle.Render("Updated "+lab.updated.In(m.loc).Format(time.Kitchen)), cardContentWidth, "  ")
		}
		cards = append(cards, labCardStyle.Render(strings.Join(lines, "\n")))
	}
	section := lipgloss.JoinVertical(lipgloss.Left, sectionTitleStyle.Render("Labs"), lipgloss.JoinVertical(lipgloss.Left, cards...))
	return section
}

func (m Model) renderLog() string {
	if len(m.log) == 0 {
		return ""
	}
	wrapped := make([]string, 0, len(m.log))
	for _, line := range m.log {
		wrapped = append(wrapped, wrapWithIndent(logStyle.Render(line), logContentWidth, "  "))
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		sectionTitleStyle.Render("Recent Events"),
		logStyle.Render(strings.Join(wrapped, "\n")),
	)
}

func (m Model) nextScheduledRun(now time.Time) time.Time {
	if len(m.schedule) == 0 {
		return time.Time{}
	}
	var candidate time.Time
	for _, s := range m.schedule {
		parsed, err := time.ParseInLocation("15:04", s, m.loc)
		if err != nil {
			continue
		}
		run := time.Date(now.Year(), now.Month(), now.Day(), parsed.Hour(), parsed.Minute(), 0, 0, m.loc)
		if !run.After(now) {
			run = run.Add(24 * time.Hour)
		}
		if candidate.IsZero() || run.Before(candidate) {
			candidate = run
		}
	}
	return candidate
}

var (
	titleStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true).Underline(true)
	sectionTitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("141")).Bold(true)
	metaLabelStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Bold(true)
	footerStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	infoStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("249"))
	detailStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("180"))
	timestampStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	logStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))
	errorStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	labTitleStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	labCardStyle      = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("57")).Padding(1, 2).MarginTop(1)
)

func statusStyle(status string) lipgloss.Style {
	base := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	s := strings.ToLower(status)
	switch s {
	case "normal":
		return base.Foreground(lipgloss.Color("120")).Bold(true)
	case "watch":
		return base.Foreground(lipgloss.Color("214")).Bold(true)
	case "alert":
		return base.Foreground(lipgloss.Color("203")).Bold(true)
	case "running":
		return base.Foreground(lipgloss.Color("39")).Bold(true)
	case "idle":
		return base.Foreground(lipgloss.Color("244")).Bold(true)
	default:
		return base
	}
}

func appendWrappedIndent(lines []string, text string, width int, indent string) []string {
	if text == "" {
		return lines
	}
	wrapped := wrapWithIndent(text, width, indent)
	return append(lines, strings.Split(wrapped, "\n")...)
}

func wrapWithIndent(text string, width int, indent string) string {
	if text == "" {
		return ""
	}
	if width <= 0 {
		width = lipgloss.Width(text)
	}
	wrapped := wordwrap.String(text, width)
	if indent == "" {
		return wrapped
	}
	lines := strings.Split(wrapped, "\n")
	if len(lines) == 0 {
		return ""
	}
	for i := 1; i < len(lines); i++ {
		lines[i] = indent + strings.TrimLeft(lines[i], " ")
	}
	return strings.Join(lines, "\n")
}
