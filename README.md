# Lab Monitor

Lab Monitor is a Go service that pulls temperature and humidity readings from ThingSpeak, summarizes recent conditions, requests an assessment from OpenAI, and emails a twice-daily report via AWS SES.

## Prerequisites
- Go 1.22 or newer
- Verified AWS SES sender email (default region: `us-east-2`, configurable via `AWS_REGION_SES`)
- OpenAI API access (supports GPT-4 and GPT-4o models)
- Network access to ThingSpeak, OpenAI, and AWS SES endpoints

## Configuration
1. Copy `config.example.yaml` to `config.yaml` and edit the following sections:
   - **`schedule`**: Local times for the two daily runs (default `06:00` and `16:00`).
   - **`channels`**: ThingSpeak channel IDs and data field names for each lab.
   - **`openai.model`**: Target model (e.g., `gpt-4o-mini-2025-01-07`, `gpt-4o`, `gpt-4-turbo`).
   - **`email.sender` / `email.recipients`**: SES-verified sender and list of recipients.
   - **`state`**: Directory for cached report history.
2. Optionally create `config.local.yaml` (gitignored) for machine-specific overrides.

## Environment Variables
Copy `.envrc.example` to `.envrc` and fill in your actual values:
```bash
cp .envrc.example .envrc
# Edit .envrc with your credentials
source .envrc
```

Required environment variables:
- `OPENAI_API_KEY` - Your OpenAI API key
- `AWS_ACCESS_KEY_ID_SES` - AWS access key for SES
- `AWS_SECRET_ACCESS_KEY_SES` - AWS secret key for SES
- `AWS_REGION_SES` - (Optional) AWS region for SES (default: `us-east-2`)

**Important**: Never commit `.envrc` or `config.yaml` to version control. These files are excluded in `.gitignore`.

## Running Locally
```bash
# Build binary
go build -o bin/labmonitor ./cmd/labmonitor

# Run directly
go run ./cmd/labmonitor -config config.yaml

# Dry-run mode (no emails sent, output to stdout)
go run ./cmd/labmonitor -config config.yaml -dry-run
```
The service performs an immediate run on start and schedules future runs at the configured times. Logs are written to stdout.

### Command-Line Flags
- `-config <path>` - Path to config file (default: `config.yaml`)
- `-dry-run` - Run without sending emails; print reports to stdout instead

### Troubleshooting
If you get "OPENAI_API_KEY is not set" even though the variable is set, see [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for solutions.

Quick fix:
```bash
# Make sure to use 'export'
export OPENAI_API_KEY="your-key-here"
go run ./cmd/labmonitor -dry-run
```

## State & History
Reports and prior model responses are stored in the directory specified by `state.directory`. Retain these files to preserve context for future assessments.

## Deploying
- Ensure the host remains online and can reach ThingSpeak, OpenAI, and SES.
- Manage the process with a supervisor (systemd, launchd, etc.) to keep the scheduler active.
- Monitor logs for failures such as API errors or SES delivery issues.

## Testing
Run the comprehensive test suite:
```bash
# Run all tests
go test ./...

# Run with verbose output
go test ./... -v

# Run with coverage
go test ./... -cover
```

Tests cover:
- Configuration validation and loading
- ThingSpeak API data parsing
- Report aggregation and statistics
- State store persistence and trimming

## Development Notes
- Run `go fmt ./...` before committing.
- Run `go test ./...` to ensure all tests pass.
- `go build ./...` verifies dependencies and compilation.

## Features

### Intelligent Context Escalation
The AI can request additional historical data if needed for proper analysis. The system automatically fetches extended time windows (up to 3 iterations) when the model determines more context is necessary.

### State Persistence
- Maintains history of the last N reports (configurable via `state.history_per_lab`)
- Each new report includes context from previous reports
- Helps identify trends over time ("improving" vs "deteriorating")

### Comprehensive Statistics
Automatically computes for each lab:
- Min/max/mean values for temperature and humidity
- Delta (change over time window)
- Latest readings vs. averages
- Sample counts

### Dual Report Formats
- **Plain text**: Clean, readable format for email clients
- **HTML**: Rich formatting with tables for visual presentation

### Dry-Run Mode
Test the entire pipeline without sending emails:
```bash
go run ./cmd/labmonitor -dry-run
```
Perfect for:
- Testing configuration changes
- Verifying ThingSpeak connectivity
- Previewing report output
- Debugging AI responses
