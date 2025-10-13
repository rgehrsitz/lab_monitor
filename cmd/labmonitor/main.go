package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"labmonitor/internal/app"
	"labmonitor/internal/config"
	"labmonitor/internal/email"
	"labmonitor/internal/events"
	"labmonitor/internal/llm"
	"labmonitor/internal/report"
	"labmonitor/internal/scheduler"
	"labmonitor/internal/state"
	"labmonitor/internal/thingspeak"
	"labmonitor/internal/ui/dashboard"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	dryRun := flag.Bool("dry-run", false, "run without sending emails (print to stdout instead)")
	uiFlag := flag.Bool("ui", false, "show interactive terminal dashboard")
	flag.Parse()

	if *dryRun {
		log.Println("Running in DRY RUN mode - emails will not be sent")
	}

	log.Printf("Loading configuration from %s...", *configPath)
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	log.Println("Initializing services...")
	tsClient := thingspeak.NewClient()
	agg := report.NewAggregator()
	pb := report.NewPromptBuilder()

	llmClient, err := llm.NewClient()
	if err != nil {
		log.Fatalf("init llm client: %v", err)
	}
	llmClient.SetModel(cfg.OpenAI.Model)
	log.Printf("Using AI model: %s", cfg.OpenAI.Model)

	emailSender, err := email.NewSender(context.Background(), cfg.Email.Sender, cfg.Email.SubjectPrefix, *dryRun)
	if err != nil {
		log.Fatalf("init email sender: %v", err)
	}

	store, err := state.NewStore(cfg.State.Directory, cfg.State.HistoryPerLab)
	if err != nil {
		log.Fatalf("init state store: %v", err)
	}

	loc := time.Local
	var (
		eventsCh  chan events.Event
		uiProgram *tea.Program
		uiDone    chan struct{}
	)
	if *uiFlag {
		eventsCh = make(chan events.Event, 128)
		model := dashboard.NewModel(cfg, *dryRun, eventsCh, loc)
		uiProgram = tea.NewProgram(model, tea.WithAltScreen())
		uiDone = make(chan struct{})
	}

	svc := app.NewService(cfg, tsClient, agg, pb, llmClient, emailSender, store, eventsCh)

	sch, err := scheduler.New(loc)
	if err != nil {
		log.Fatalf("init scheduler: %v", err)
	}
	log.Println("Services initialized successfully")

	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if uiProgram != nil {
		go func() {
			if err := uiProgram.Start(); err != nil {
				log.Printf("ui error: %v", err)
			}
			stop()
			close(uiDone)
		}()
	}

	times := []string{cfg.Schedule.First, cfg.Schedule.Second}
	job := func(jobCtx context.Context) error {
		runCtx, cancel := context.WithTimeout(jobCtx, 3*time.Minute)
		defer cancel()
		return svc.Run(runCtx, time.Now())
	}
	if err := sch.ScheduleDaily(times, func(ctx context.Context) error {
		err := job(ctx)
		if err != nil {
			log.Printf("scheduled run failed: %v", err)
		}
		return err
	}); err != nil {
		log.Fatalf("schedule jobs: %v", err)
	}

	schedulerDone := make(chan struct{})
	go func() {
		if err := sch.Start(signalCtx); err != nil {
			log.Printf("scheduler stopped with error: %v", err)
		}
		close(schedulerDone)
	}()

	log.Println("Running initial assessment...")
	if err := job(signalCtx); err != nil {
		log.Printf("initial run failed: %v", err)
	}

	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("Lab Monitor is running")
	log.Printf("Scheduled runs: %s and %s daily", cfg.Schedule.First, cfg.Schedule.Second)
	log.Printf("Monitoring %d lab(s)", len(cfg.Channels))
	for _, ch := range cfg.Channels {
		log.Printf("  - %s (Channel ID: %d)", ch.Name, ch.ID)
	}
	log.Println("Press Ctrl+C to stop")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	<-signalCtx.Done()
	log.Println("Shutting down...")
	<-schedulerDone
	if eventsCh != nil {
		close(eventsCh)
		if uiDone != nil {
			<-uiDone
		}
	}
	fmt.Println("shutdown complete")
}
