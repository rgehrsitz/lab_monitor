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
}

type EmailConfig struct {
	Sender        string   `yaml:"sender"`
	Recipients    []string `yaml:"recipients"`
	SubjectPrefix string   `yaml:"subject_prefix"`
}

type StateConfig struct {
	Directory     string `yaml:"directory"`
	HistoryPerLab int    `yaml:"history_per_lab"`
}

type Config struct {
	Schedule ScheduleConfig  `yaml:"schedule"`
	Channels []ChannelConfig `yaml:"channels"`
	OpenAI   OpenAIConfig    `yaml:"openai"`
	Email    EmailConfig     `yaml:"email"`
	State    StateConfig     `yaml:"state"`
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
	if len(c.Email.Recipients) == 0 {
		return fmt.Errorf("at least one email recipient must be configured")
	}
	if c.State.Directory == "" {
		return fmt.Errorf("state directory must be configured")
	}
	if c.State.HistoryPerLab <= 0 {
		return fmt.Errorf("state history_per_lab must be > 0")
	}
	return nil
}
