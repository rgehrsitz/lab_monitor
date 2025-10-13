package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"time"

	"labmonitor/internal/app"
	"labmonitor/internal/config"
	"labmonitor/internal/email"
	"labmonitor/internal/llm"
	"labmonitor/internal/report"
	"labmonitor/internal/scheduler"
	"labmonitor/internal/state"
	"labmonitor/internal/thingspeak"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	dryRun := flag.Bool("dry-run", false, "run without sending emails (print to stdout instead)")
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

	svc := app.NewService(cfg, tsClient, agg, pb, llmClient, emailSender, store)

	loc := time.Local
	sch, err := scheduler.New(loc)
	if err != nil {
		log.Fatalf("init scheduler: %v", err)
	}
	log.Println("Services initialized successfully")

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

	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := sch.Start(signalCtx); err != nil {
			log.Printf("scheduler stopped with error: %v", err)
		}
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
	fmt.Println("shutdown complete")
}
