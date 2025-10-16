package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type ScheduleConfig struct {
	Times []string `yaml:"times"`
}

type ChannelConfig struct {
	ID               int    `yaml:"id"`
	Name             string `yaml:"name"`
	TemperatureField string `yaml:"temperature_field"`
	HumidityField    string `yaml:"humidity_field"`
}

type OpenAIConfig struct {
	Model string `yaml:"model"`
	// MaxContextAttempts limits how many back-and-forth rounds the model can
	// request additional historical context within a single run. If 0, a
	// sensible default is applied by the service.
	MaxContextAttempts int `yaml:"max_context_attempts"`
	// InitialTimeoutSeconds caps duration of the initial (canonical) assessment
	// LLM call. If 0, a default (e.g. 45s) is applied.
	InitialTimeoutSeconds int `yaml:"initial_timeout_seconds"`
	// ExtensionTimeoutSeconds caps duration of any extended (need-context)
	// assessment re-analysis call. If 0, a default (e.g. 25s) is applied.
	ExtensionTimeoutSeconds int `yaml:"extension_timeout_seconds"`
	// RequestTimeoutSeconds caps duration of tone transform or other shorter
	// auxiliary LLM calls. If 0, a default (e.g. 20s) is applied.
	RequestTimeoutSeconds int `yaml:"request_timeout_seconds"`
	// ToneTimeoutSeconds caps duration of per-profile tone transform calls. If 0 uses RequestTimeoutSeconds or its default.
	ToneTimeoutSeconds int `yaml:"tone_timeout_seconds"`
	// MaxRetries controls how many retries are attempted for retryable
	// transient LLM errors (429, 5xx, timeouts). If <0 treated as 0.
	MaxRetries int `yaml:"max_retries"`
	// RetryBackoffMs is the base backoff in milliseconds for exponential
	// backoff with jitter. If 0, a default (e.g. 400ms) is used.
	RetryBackoffMs int `yaml:"retry_backoff_ms"`
}

type EmailConfig struct {
	Sender        string `yaml:"sender"`
	SubjectPrefix string `yaml:"subject_prefix"`
	// Profiles define grouped recipients sharing a style profile. If set,
	// the service will generate one canonical assessment then transform text
	// per profile instead of per individual recipient.
	Profiles []EmailProfile `yaml:"profiles"`
}

type StateConfig struct {
	Directory     string `yaml:"directory"`
	HistoryPerLab int    `yaml:"history_per_lab"`
}

// StyleConfig controls tone and rendering options for reports.
type StyleConfig struct {
	// Personality controls the overall tone: "neutral" (default), "friendly",
	// "snarky", "humorous". The prompt will be adjusted accordingly.
	Personality string `yaml:"personality"`
	// SnarkLevel 0-3 scales how cheeky/snarky the language can be when
	// personality is snarky or humorous. 0 = off, 1 = light, 2 = medium, 3 = spicy.
	SnarkLevel int `yaml:"snark_level"`
	// UseIcons enables emoji icons in HTML output to convey status.
	UseIcons bool `yaml:"use_icons"`
	// UseColor enables colored badges and highlights in HTML output.
	UseColor bool `yaml:"use_color"`
}

type Config struct {
	Schedule ScheduleConfig  `yaml:"schedule"`
	Channels []ChannelConfig `yaml:"channels"`
	OpenAI   OpenAIConfig    `yaml:"openai"`
	Email    EmailConfig     `yaml:"email"`
	State    StateConfig     `yaml:"state"`
	Style    StyleConfig     `yaml:"style"`
}

// EmailProfile groups recipients under a shared style personality.
type EmailProfile struct {
	Name        string   `yaml:"name"`
	Recipients  []string `yaml:"recipients"`
	Personality string   `yaml:"personality"`
	SnarkLevel  int      `yaml:"snark_level"`
	UseIcons    *bool    `yaml:"use_icons,omitempty"`
	UseColor    *bool    `yaml:"use_color,omitempty"`
}

func Load(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Validate reuses internal validation for callers that manipulate Config directly.
func (c Config) Validate() error {
	return c.validate()
}

func (c Config) validate() error {
	if len(c.Schedule.Times) == 0 {
		return fmt.Errorf("at least one schedule time must be provided")
	}
	if len(c.Channels) == 0 {
		return fmt.Errorf("at least one channel must be configured")
	}
	for _, ch := range c.Channels {
		if ch.ID == 0 {
			return fmt.Errorf("channel id missing for %s", ch.Name)
		}
		if ch.TemperatureField == "" || ch.HumidityField == "" {
			return fmt.Errorf("channel %s must specify temperature_field and humidity_field", ch.Name)
		}
	}
	if c.OpenAI.Model == "" {
		return fmt.Errorf("openai model must be configured")
	}
	if c.Email.Sender == "" {
		return fmt.Errorf("email sender must be configured")
	}
	if len(c.Email.Profiles) == 0 {
		return fmt.Errorf("at least one email profile must be configured")
	}
	// Basic profile validation
	for _, p := range c.Email.Profiles {
		if p.Name == "" {
			return fmt.Errorf("email profile name is required")
		}
		if len(p.Recipients) == 0 {
			return fmt.Errorf("email profile %s must have at least one recipient", p.Name)
		}
	}
	if c.State.Directory == "" {
		return fmt.Errorf("state directory must be configured")
	}
	if c.State.HistoryPerLab <= 0 {
		return fmt.Errorf("state history_per_lab must be > 0")
	}
	// Soft validation / normalization of OpenAI timeout & retry fields (allow 0 for defaults)
	if c.OpenAI.MaxRetries < 0 {
		return fmt.Errorf("openai max_retries must be >= 0")
	}
	if c.OpenAI.InitialTimeoutSeconds < 0 || c.OpenAI.ExtensionTimeoutSeconds < 0 || c.OpenAI.RequestTimeoutSeconds < 0 || c.OpenAI.ToneTimeoutSeconds < 0 {
		return fmt.Errorf("openai timeouts must be >= 0 seconds")
	}
	if c.OpenAI.RetryBackoffMs < 0 {
		return fmt.Errorf("openai retry_backoff_ms must be >= 0")
	}
	return nil
}
