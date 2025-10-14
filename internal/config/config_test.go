package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfig_Validate_Success(t *testing.T) {
	cfg := Config{Schedule: ScheduleConfig{Times: []string{"06:00", "16:00"}}, Channels: []ChannelConfig{{ID: 123, Name: "Lab A", TemperatureField: "t", HumidityField: "h"}}, OpenAI: OpenAIConfig{Model: "gpt-4o"}, Email: EmailConfig{Sender: "sender@example.com", Profiles: []EmailProfile{{Name: "default", Recipients: []string{"r@example.com"}}}}, State: StateConfig{Directory: "/tmp", HistoryPerLab: 3}}
	if err := cfg.validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfig_Validate_NoProfiles(t *testing.T) {
	cfg := Config{Schedule: ScheduleConfig{Times: []string{"06:00"}}, Channels: []ChannelConfig{{ID: 1, Name: "Lab", TemperatureField: "t", HumidityField: "h"}}, OpenAI: OpenAIConfig{Model: "gpt-4o"}, Email: EmailConfig{Sender: "sender@example.com"}, State: StateConfig{Directory: "/tmp", HistoryPerLab: 3}}
	if err := cfg.validate(); err == nil {
		t.Error("expected error for missing profiles")
	}
}

func TestConfig_Validate_ProfileEmptyRecipients(t *testing.T) {
	cfg := Config{Schedule: ScheduleConfig{Times: []string{"06:00"}}, Channels: []ChannelConfig{{ID: 1, Name: "Lab", TemperatureField: "t", HumidityField: "h"}}, OpenAI: OpenAIConfig{Model: "gpt-4o"}, Email: EmailConfig{Sender: "sender@example.com", Profiles: []EmailProfile{{Name: "p1", Recipients: []string{}}}}, State: StateConfig{Directory: "/tmp", HistoryPerLab: 3}}
	if err := cfg.validate(); err == nil {
		t.Error("expected error for empty recipients in profile")
	}
}

func TestConfig_Load_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `schedule:
  times: ["06:00"]
channels:
  - id: 5
    name: "Env"
    temperature_field: "t"
    humidity_field: "h"
openai:
  model: gpt-4o
email:
  sender: sender@example.com
  profiles:
    - name: default
      recipients: ["r@example.com"]
state:
  directory: state
  history_per_lab: 3
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("load: %v", err)
	}
}

func TestConfig_OpenAI_InvalidTimeouts(t *testing.T) {
	cfg := Config{Schedule: ScheduleConfig{Times: []string{"06:00"}}, Channels: []ChannelConfig{{ID: 1, Name: "Lab", TemperatureField: "t", HumidityField: "h"}}, OpenAI: OpenAIConfig{Model: "gpt-4o", InitialTimeoutSeconds: -1}, Email: EmailConfig{Sender: "sender@example.com", Profiles: []EmailProfile{{Name: "p", Recipients: []string{"a@example.com"}}}}, State: StateConfig{Directory: "/tmp", HistoryPerLab: 2}}
	if err := cfg.validate(); err == nil {
		t.Error("expected error for negative initial timeout")
	}
}

func TestConfig_OpenAI_InvalidRetries(t *testing.T) {
	cfg := Config{Schedule: ScheduleConfig{Times: []string{"06:00"}}, Channels: []ChannelConfig{{ID: 1, Name: "Lab", TemperatureField: "t", HumidityField: "h"}}, OpenAI: OpenAIConfig{Model: "gpt-4o", MaxRetries: -2}, Email: EmailConfig{Sender: "sender@example.com", Profiles: []EmailProfile{{Name: "p", Recipients: []string{"a@example.com"}}}}, State: StateConfig{Directory: "/tmp", HistoryPerLab: 2}}
	if err := cfg.validate(); err == nil {
		t.Error("expected error for negative retries")
	}
}

func TestConfig_OpenAI_InvalidToneTimeout(t *testing.T) {
	cfg := Config{Schedule: ScheduleConfig{Times: []string{"06:00"}}, Channels: []ChannelConfig{{ID: 1, Name: "Lab", TemperatureField: "t", HumidityField: "h"}}, OpenAI: OpenAIConfig{Model: "gpt-4o", ToneTimeoutSeconds: -5}, Email: EmailConfig{Sender: "sender@example.com", Profiles: []EmailProfile{{Name: "p", Recipients: []string{"a@example.com"}}}}, State: StateConfig{Directory: "/tmp", HistoryPerLab: 2}}
	if err := cfg.validate(); err == nil {
		t.Error("expected error for negative tone timeout")
	}
}
