package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"cloud.google.com/go/firestore"

	"labmonitor/internal/configstore"
	"labmonitor/internal/server"
	"labmonitor/internal/state"
	"labmonitor/internal/storage"
)

func main() {
	var (
		listenAddr    = flag.String("listen", ":8080", "HTTP listen address")
		configBackend = flag.String("config-backend", "file", "configuration backend: file or firestore")
		reportBackend = flag.String("report-backend", "file", "report backend: file or firestore")
		configPath    = flag.String("config-path", "config.yaml", "path to local YAML config (file backend)")
		stateDir      = flag.String("state-dir", "state", "directory for report history when using file backend")
		projectID     = flag.String("gcp-project", os.Getenv("GOOGLE_CLOUD_PROJECT"), "GCP project ID (firestore backend)")
		dryRun        = flag.Bool("dry-run", false, "do not send emails (logs instead)")
		runTimeout    = flag.Duration("run-timeout", 4*time.Minute, "maximum duration of a single run")
		historyLimit  = flag.Int("history-limit", 30, "maximum number of report records to retain")
	)
	flag.Parse()

	ctx := context.Background()

	var (
		fsClient *firestore.Client
		err      error
	)

	needFirestore := strings.EqualFold(*configBackend, "firestore") || strings.EqualFold(*reportBackend, "firestore")
	if needFirestore {
		if *projectID == "" {
			log.Fatal("gcp project required when using firestore backend")
		}
		fsClient, err = firestore.NewClient(ctx, *projectID)
		if err != nil {
			log.Fatalf("init firestore client: %v", err)
		}
		defer fsClient.Close()
	}

	cfgStore, err := buildConfigStore(*configBackend, *configPath, fsClient)
	if err != nil {
		log.Fatalf("config store: %v", err)
	}

	reportStore, err := buildReportStore(*reportBackend, *stateDir, *historyLimit, fsClient)
	if err != nil {
		log.Fatalf("report store: %v", err)
	}

	srv, err := server.New(server.Options{
		ConfigStore: cfgStore,
		ReportStore: reportStore,
		DryRun:      *dryRun,
		RunTimeout:  *runTimeout,
	})
	if err != nil {
		log.Fatalf("init server: %v", err)
	}

	httpServer := &http.Server{
		Addr:    *listenAddr,
		Handler: loggingMiddleware(srv),
	}

	go func() {
		log.Printf("listening on %s", *listenAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown error: %v", err)
	}
	log.Println("server stopped")
}

func buildConfigStore(backend, path string, client *firestore.Client) (configstore.Store, error) {
	switch strings.ToLower(backend) {
	case "file":
		return configstore.NewFileStore(path), nil
	case "firestore":
		if client == nil {
			return nil, fmt.Errorf("firestore client required")
		}
		return configstore.NewFirestoreStore(client), nil
	default:
		return nil, fmt.Errorf("unsupported config backend: %s", backend)
	}
}

func buildReportStore(backend, stateDir string, history int, client *firestore.Client) (storage.ReportStore, error) {
	switch strings.ToLower(backend) {
	case "file":
		store, err := state.NewStore(stateDir, history)
		if err != nil {
			return nil, err
		}
		return store, nil
	case "firestore":
		if client == nil {
			return nil, fmt.Errorf("firestore client required")
		}
		return storage.NewFirestoreReportStore(client, history), nil
	default:
		return nil, fmt.Errorf("unsupported report backend: %s", backend)
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}
