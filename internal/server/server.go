package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"labmonitor/internal/app"
	"labmonitor/internal/config"
	"labmonitor/internal/configstore"
	"labmonitor/internal/email"
	"labmonitor/internal/events"
	"labmonitor/internal/llm"
	"labmonitor/internal/report"
	"labmonitor/internal/storage"
	"labmonitor/internal/thingspeak"
)

// Options configure the HTTP server.
type Options struct {
	ConfigStore   configstore.Store
	ReportStore   storage.ReportStore
	Authenticator Authenticator
	DryRun        bool
	RunTimeout    time.Duration
}

type Server struct {
	configStore   configstore.Store
	reportStore   storage.ReportStore
	authenticator Authenticator
	dryRun        bool
	runTimeout    time.Duration

	mux    *http.ServeMux
	runMu  sync.Mutex
	active *RunStatus
	last   *RunStatus
}

type RunStatus struct {
	ID            string         `json:"id"`
	State         string         `json:"state"`
	StartedAt     time.Time      `json:"started_at"`
	CompletedAt   *time.Time     `json:"completed_at,omitempty"`
	TriggeredBy   string         `json:"triggered_by,omitempty"`
	ConfigVersion string         `json:"config_version,omitempty"`
	Error         string         `json:"error,omitempty"`
	Events        []events.Event `json:"events,omitempty"`
}

func New(opts Options) (*Server, error) {
	if opts.ConfigStore == nil {
		return nil, errors.New("config store required")
	}
	if opts.ReportStore == nil {
		return nil, errors.New("report store required")
	}
	auth := opts.Authenticator
	if auth == nil {
		auth = NoopAuthenticator{}
	}
	timeout := opts.RunTimeout
	if timeout <= 0 {
		timeout = 4 * time.Minute
	}

	s := &Server{
		configStore:   opts.ConfigStore,
		reportStore:   opts.ReportStore,
		authenticator: auth,
		dryRun:        opts.DryRun,
		runTimeout:    timeout,
		mux:           http.NewServeMux(),
	}
	s.routes()
	return s, nil
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.Handle("/api/status", s.authenticated(s.handleStatus))
	s.mux.Handle("/api/run", s.authenticated(s.handleRun))
	s.mux.Handle("/api/config", s.authenticated(s.handleConfig))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) authenticated(next func(http.ResponseWriter, *http.Request, *User)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := s.authenticator.Authenticate(r)
		if err != nil {
			http.Error(w, fmt.Sprintf("auth failed: %v", err), http.StatusUnauthorized)
			return
		}
		next(w, r, user)
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request, user *User) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	cfg, version, err := s.configStore.Get(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("load config: %v", err), http.StatusInternalServerError)
		return
	}

	s.runMu.Lock()
	active := s.active
	last := s.last
	s.runMu.Unlock()

	resp := map[string]any{
		"config_version": version,
		"schedule":       cfg.Schedule,
		"channels":       cfg.Channels,
		"email": map[string]any{
			"sender":         cfg.Email.Sender,
			"subject_prefix": cfg.Email.SubjectPrefix,
			"profiles":       cfg.Email.Profiles,
		},
		"style":    cfg.Style,
		"run":      active,
		"last_run": last,
	}
	s.writeJSON(w, resp)
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request, user *User) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if user == nil || !user.IsAdmin {
		http.Error(w, "admin privileges required", http.StatusForbidden)
		return
	}

	status, err := s.startRun(user.Email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	s.writeJSON(w, status)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request, user *User) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		cfg, version, err := s.configStore.Get(ctx)
		if err != nil {
			http.Error(w, fmt.Sprintf("load config: %v", err), http.StatusInternalServerError)
			return
		}
		s.writeJSON(w, map[string]any{"config": cfg, "version": version})
	case http.MethodPut:
		if user == nil || !user.IsAdmin {
			http.Error(w, "admin privileges required", http.StatusForbidden)
			return
		}
		var payload struct {
			Config  config.Config `json:"config"`
			Version string        `json:"version"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, fmt.Sprintf("invalid payload: %v", err), http.StatusBadRequest)
			return
		}
		if err := s.configStore.Put(ctx, payload.Config, payload.Version); err != nil {
			var conflict configstore.ErrConflict
			if errors.As(err, &conflict) {
				http.Error(w, conflict.Error(), http.StatusConflict)
				return
			}
			http.Error(w, fmt.Sprintf("store config: %v", err), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPut)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) startRun(triggeredBy string) (*RunStatus, error) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.active != nil && s.active.State == "running" {
		return nil, fmt.Errorf("run already in progress")
	}

	status := &RunStatus{
		ID:          uuid.NewString(),
		State:       "running",
		StartedAt:   time.Now().UTC(),
		TriggeredBy: triggeredBy,
	}
	s.active = status
	go s.executeRun(status.ID)
	return status, nil
}

func (s *Server) executeRun(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.runTimeout)
	defer cancel()

	cfg, version, err := s.configStore.Get(ctx)
	if err != nil {
		s.finishRun(id, nil, fmt.Errorf("load config: %w", err))
		return
	}

	eventsCh := make(chan events.Event, 256)
	var eventsLog []events.Event
	var eventsMu sync.Mutex
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range eventsCh {
			eventsMu.Lock()
			eventsLog = append(eventsLog, ev)
			if len(eventsLog) > 200 {
				eventsLog = append([]events.Event(nil), eventsLog[len(eventsLog)-200:]...)
			}
			snapshot := append([]events.Event(nil), eventsLog...)
			eventsMu.Unlock()
			s.updateRunEvents(id, snapshot)
		}
	}()

	svc, err := s.buildService(cfg, eventsCh)
	if err != nil {
		close(eventsCh)
		<-done
		s.finishRun(id, nil, err)
		return
	}

	s.setRunConfigVersion(id, version)

	runErr := svc.Run(ctx, time.Now())
	close(eventsCh)
	<-done

	s.finishRun(id, eventsLog, runErr)
}

func (s *Server) buildService(cfg config.Config, eventsCh chan<- events.Event) (*app.Service, error) {
	tsClient := thingspeak.NewClient()
	agg := report.NewAggregator()
	pb := report.NewPromptBuilderWithStyle(report.Style{
		Personality: cfg.Style.Personality,
		SnarkLevel:  cfg.Style.SnarkLevel,
	})

	llmClient, err := llm.NewClient()
	if err != nil {
		return nil, fmt.Errorf("init llm client: %w", err)
	}
	llmClient.SetModel(cfg.OpenAI.Model)

	emailSender, err := email.NewSender(context.Background(), cfg.Email.Sender, cfg.Email.SubjectPrefix, s.dryRun)
	if err != nil {
		return nil, fmt.Errorf("init email sender: %w", err)
	}

	return app.NewService(cfg, tsClient, agg, pb, llmClient, emailSender, s.reportStore, eventsCh), nil
}

func (s *Server) updateRunEvents(id string, events []events.Event) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.active != nil && s.active.ID == id {
		s.active.Events = events
	}
}

func (s *Server) setRunConfigVersion(id, version string) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.active != nil && s.active.ID == id {
		s.active.ConfigVersion = version
	}
}

func (s *Server) finishRun(id string, events []events.Event, runErr error) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.active == nil || s.active.ID != id {
		return
	}
	now := time.Now().UTC()
	s.active.CompletedAt = &now
	s.active.Events = events
	if runErr != nil {
		s.active.State = "failed"
		s.active.Error = runErr.Error()
	} else {
		s.active.State = "succeeded"
		s.active.Error = ""
	}
	s.last = s.active
	s.active = nil
}

func (s *Server) writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		log.Printf("write json: %v", err)
	}
}
