package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfig_Validate_Success(t *testing.T) {
	cfg := Config{
		Schedule: ScheduleConfig{
			Times: []string{"06:00", "16:00"},
		},
		Channels: []ChannelConfig{
			{
				ID:               12345,
				Name:             "Lab A",
				TemperatureField: "field1",
				HumidityField:    "field2",
			},
		},
		OpenAI: OpenAIConfig{
			Model: "gpt-4o",
		},
		Email: EmailConfig{
			Sender:        "sender@example.com",
			Recipients:    []string{"recipient@example.com"},
			SubjectPrefix: "[Test]",
		},
		State: StateConfig{
			Directory:     "/tmp/state",
			HistoryPerLab: 5,
		},
	}

	if err := cfg.validate(); err != nil {
		t.Errorf("Expected valid config to pass validation, got error: %v", err)
	}
}

func TestConfig_Validate_MissingSchedule(t *testing.T) {
	cfg := Config{
		Schedule: ScheduleConfig{
			Times: []string{},
		},
		Channels: []ChannelConfig{
			{ID: 1, Name: "Lab A", TemperatureField: "field1", HumidityField: "field2"},
		},
		OpenAI: OpenAIConfig{Model: "gpt-4o"},
		Email: EmailConfig{
			Sender:     "sender@example.com",
			Recipients: []string{"recipient@example.com"},
		},
		State: StateConfig{Directory: "/tmp", HistoryPerLab: 5},
	}

	if err := cfg.validate(); err == nil {
		t.Error("Expected error for missing schedule time, got nil")
	}
}

func TestConfig_Validate_NoChannels(t *testing.T) {
	cfg := Config{
		Schedule: ScheduleConfig{Times: []string{"06:00", "16:00"}},
		Channels: []ChannelConfig{},
		OpenAI:   OpenAIConfig{Model: "gpt-4o"},
		Email: EmailConfig{
			Sender:     "sender@example.com",
			Recipients: []string{"recipient@example.com"},
		},
		State: StateConfig{Directory: "/tmp", HistoryPerLab: 5},
	}

	if err := cfg.validate(); err == nil {
		t.Error("Expected error for no channels, got nil")
	}
}

func TestConfig_Validate_ChannelMissingID(t *testing.T) {
	cfg := Config{
		Schedule: ScheduleConfig{Times: []string{"06:00", "16:00"}},
		Channels: []ChannelConfig{
			{
				ID:               0,
				Name:             "Lab A",
				TemperatureField: "field1",
				HumidityField:    "field2",
			},
		},
		OpenAI: OpenAIConfig{Model: "gpt-4o"},
		Email: EmailConfig{
			Sender:     "sender@example.com",
			Recipients: []string{"recipient@example.com"},
		},
		State: StateConfig{Directory: "/tmp", HistoryPerLab: 5},
	}

	if err := cfg.validate(); err == nil {
		t.Error("Expected error for channel missing ID, got nil")
	}
}

func TestConfig_Validate_ChannelMissingFields(t *testing.T) {
	cfg := Config{
		Schedule: ScheduleConfig{Times: []string{"06:00", "16:00"}},
		Channels: []ChannelConfig{
			{
				ID:               12345,
				Name:             "Lab A",
				TemperatureField: "",
				HumidityField:    "field2",
			},
		},
		OpenAI: OpenAIConfig{Model: "gpt-4o"},
		Email: EmailConfig{
			Sender:     "sender@example.com",
			Recipients: []string{"recipient@example.com"},
		},
		State: StateConfig{Directory: "/tmp", HistoryPerLab: 5},
	}

	if err := cfg.validate(); err == nil {
		t.Error("Expected error for channel missing temperature field, got nil")
	}
}

func TestConfig_Validate_MissingOpenAIModel(t *testing.T) {
	cfg := Config{
		Schedule: ScheduleConfig{Times: []string{"06:00", "16:00"}},
		Channels: []ChannelConfig{
			{ID: 1, Name: "Lab A", TemperatureField: "field1", HumidityField: "field2"},
		},
		OpenAI: OpenAIConfig{Model: ""},
		Email: EmailConfig{
			Sender:     "sender@example.com",
			Recipients: []string{"recipient@example.com"},
		},
		State: StateConfig{Directory: "/tmp", HistoryPerLab: 5},
	}

	if err := cfg.validate(); err == nil {
		t.Error("Expected error for missing OpenAI model, got nil")
	}
}

func TestConfig_Validate_MissingEmailSender(t *testing.T) {
	cfg := Config{
		Schedule: ScheduleConfig{Times: []string{"06:00", "16:00"}},
		Channels: []ChannelConfig{
			{ID: 1, Name: "Lab A", TemperatureField: "field1", HumidityField: "field2"},
		},
		OpenAI: OpenAIConfig{Model: "gpt-4o"},
		Email: EmailConfig{
			Sender:     "",
			Recipients: []string{"recipient@example.com"},
		},
		State: StateConfig{Directory: "/tmp", HistoryPerLab: 5},
	}

	if err := cfg.validate(); err == nil {
		t.Error("Expected error for missing email sender, got nil")
	}
}

func TestConfig_Validate_NoEmailRecipients(t *testing.T) {
	cfg := Config{
		Schedule: ScheduleConfig{Times: []string{"06:00", "16:00"}},
		Channels: []ChannelConfig{
			{ID: 1, Name: "Lab A", TemperatureField: "field1", HumidityField: "field2"},
		},
		OpenAI: OpenAIConfig{Model: "gpt-4o"},
		Email: EmailConfig{
			Sender:     "sender@example.com",
			Recipients: []string{},
		},
		State: StateConfig{Directory: "/tmp", HistoryPerLab: 5},
	}

	if err := cfg.validate(); err == nil {
		t.Error("Expected error for no email recipients, got nil")
	}
}

func TestConfig_Validate_MissingStateDirectory(t *testing.T) {
	cfg := Config{
		Schedule: ScheduleConfig{Times: []string{"06:00", "16:00"}},
		Channels: []ChannelConfig{
			{ID: 1, Name: "Lab A", TemperatureField: "field1", HumidityField: "field2"},
		},
		OpenAI: OpenAIConfig{Model: "gpt-4o"},
		Email: EmailConfig{
			Sender:     "sender@example.com",
			Recipients: []string{"recipient@example.com"},
		},
		State: StateConfig{Directory: "", HistoryPerLab: 5},
	}

	if err := cfg.validate(); err == nil {
		t.Error("Expected error for missing state directory, got nil")
	}
}

func TestConfig_Validate_InvalidHistoryPerLab(t *testing.T) {
	cfg := Config{
		Schedule: ScheduleConfig{Times: []string{"06:00", "16:00"}},
		Channels: []ChannelConfig{
			{ID: 1, Name: "Lab A", TemperatureField: "field1", HumidityField: "field2"},
		},
		OpenAI: OpenAIConfig{Model: "gpt-4o"},
		Email: EmailConfig{
			Sender:     "sender@example.com",
			Recipients: []string{"recipient@example.com"},
		},
		State: StateConfig{Directory: "/tmp", HistoryPerLab: 0},
	}

	if err := cfg.validate(); err == nil {
		t.Error("Expected error for invalid history_per_lab, got nil")
	}
}

func TestConfig_Load_Success(t *testing.T) {
	// Create temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	configContent := `schedule:
  times:
    - "06:00"
    - "16:00"
channels:
  - id: 12345
    name: "Test Lab"
    temperature_field: "field1"
    humidity_field: "field2"
openai:
  model: "gpt-4o"
email:
  sender: "sender@example.com"
  recipients:
    - "recipient@example.com"
  subject_prefix: "[Test]"
state:
  directory: "state"
  history_per_lab: 5
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create temp config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if len(cfg.Schedule.Times) != 2 || cfg.Schedule.Times[0] != "06:00" || cfg.Schedule.Times[1] != "16:00" {
		t.Errorf("Expected schedule times [06:00, 16:00], got %v", cfg.Schedule.Times)
	}
	if len(cfg.Channels) != 1 {
		t.Errorf("Expected 1 channel, got %d", len(cfg.Channels))
	}
	if cfg.Channels[0].ID != 12345 {
		t.Errorf("Expected channel ID 12345, got %d", cfg.Channels[0].ID)
	}
	if cfg.OpenAI.Model != "gpt-4o" {
		t.Errorf("Expected model 'gpt-4o', got '%s'", cfg.OpenAI.Model)
	}
}

func TestConfig_Load_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("Expected error for non-existent config file, got nil")
	}
}

func TestConfig_Load_InvalidYAML(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "bad.yaml")

	if err := os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0644); err != nil {
		t.Fatalf("Failed to create temp config file: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("Expected error for invalid YAML, got nil")
	}
}
