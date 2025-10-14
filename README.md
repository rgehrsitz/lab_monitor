# Lab Monitor

Lab Monitor is a Go service that pulls temperature and humidity readings from ThingSpeak, summarizes recent conditions, requests an assessment from OpenAI, and emails scheduled reports via AWS SES.

## Prerequisites
- Go 1.22 or newer
- Verified AWS SES sender email (default region: `us-east-2`, configurable via `AWS_REGION_SES`)
- OpenAI API access (supports GPT-4 and GPT-4o models)
- Network access to ThingSpeak, OpenAI, and AWS SES endpoints

## Configuration
1. Copy `config.example.yaml` to `config.yaml` and edit the following sections:
   - **`schedule.times`**: Array of local times for daily runs (e.g., `["06:00", "16:00"]`). You can specify as many times as needed.
   - **`channels`**: ThingSpeak channel IDs and data field names for each lab.
   - **`openai.model`**: Target model (e.g., `gpt-4o-mini-2025-01-07`, `gpt-4o`, `gpt-4-turbo`).
   - **`openai.max_context_attempts`** (optional): How many back-and-forth rounds the model can request additional historical data within a single run. Defaults to 3 if omitted.
   - **`email.sender` / `email.profiles`**: SES-verified sender and grouped recipient profiles (see Profiles section). Legacy `email.recipients` and `email.recipients_detailed` have been removed.
   - **`state`**: Directory for cached report history.
   - **`style`** (optional): Output tone and visuals: `personality` (neutral, friendly, snarky, humorous), `snark_level` (0–3), `use_icons` (emoji), `use_color` (HTML color accents).
   - (Removed) `email.recipients_detailed`: Replaced by profile-level overrides. Migrate by grouping recipients sharing tone.
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

# Run with terminal dashboard
go run ./cmd/labmonitor -config config.yaml -ui

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
The AI can request additional historical data if needed for proper analysis. Requests in a single round are coalesced per lab (largest window wins) and cumulative per-lab expansion is capped at 7 days to bound latency and cost. Control iterations via `openai.max_context_attempts`.

Recommended values: 2–4. Higher values may increase latency and API usage without proportional benefit.

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
- **HTML**: Rich formatting with tables for visual presentation; optional icons and color accents to emphasize status

### Profiles, Personality, and Emoji
Email sending is now profile-based:

```yaml
email:
   sender: sender@example.com
   subject_prefix: "[Lab Monitor]"
   profiles:
      - name: engineering
         recipients: ["eng1@example.com", "eng2@example.com"]
         personality: snarky
         snark_level: 2
         use_icons: true
         use_color: true
      - name: management
         recipients: ["mgr@example.com"]
         personality: friendly
         use_icons: true
         use_color: true
      - name: archive
         recipients: ["archive@example.com"]  # inherits global neutral style
```

Global style (top-level `style:`) supplies defaults; each profile can override `personality`, `snark_level`, `use_icons`, `use_color`.

Tone guidelines:
- neutral: concise, professional
- friendly: supportive, upbeat
- humorous: light wit, never detracting from clarity
- snarky: dry cheekiness scaled by `snark_level` (0–3) while staying respectful

Emoji & Flair:
- The LLM may include tasteful emoji in its JSON summary/details for non-neutral personalities.
- Formatter optionally adds status icons/colors (controlled by global or per-profile `use_icons` / `use_color`).
- Avoid excessive emojis; they should reinforce meaning (e.g., alerts, improvements) not distract.

Migration Note: If you previously had `email.recipients` or `email.recipients_detailed`, create one or more `profiles` and move those addresses under `recipients:` arrays. The application now fails fast if no profiles are defined.

### Trend Metrics
Structured multi-window statistics are computed per lab and provided to the model, replacing the need to send a long tail of prior reports. Buckets: 6h, 24h, 72h, 168h (7d). A prior 24h block (24–48h ago) is compared to the current 24h mean to yield `delta_24h`.
```jsonc
{
   "trend_metrics": {
      "labs": [
         {
            "name": "Lab A",
            "temp": {"mean": {"6h": 22.1, "24h": 21.8, "72h": 21.5, "168h": 21.2}, "delta_24h": 0.3},
            "humidity": {"mean": {"6h": 41.2, "24h": 42.0, "72h": 43.5, "168h": 44.1}, "delta_24h": -0.8},
            "status_counts": {"normal": 5, "watch": 1, "alert": 0}
         }
      ]
   }
}
```

Computation:
1. Fetch raw ThingSpeak data per lab for each window (6h/24h/72h/168h) at run time.
2. Compute mean temperature & humidity per window; compute `delta_24h` as difference between current 24h mean and prior 24–48h mean.
3. Derive status counts from persisted report history.
4. Sort labs by name for deterministic prompt ordering; reuse metrics for any extended (need-context) re-analysis.

The model is instructed to lean on these bucketed metrics before requesting additional broad history.

### OpenAI Timeouts & Retries
`openai.initial_timeout_seconds`, `extension_timeout_seconds`, and `request_timeout_seconds` bound latency for canonical, extended, and tone-transform calls. Transient errors (timeouts, 429, 5xx) are retried up to `openai.max_retries` with exponential backoff (`retry_backoff_ms`). Extended assessment failures degrade gracefully (the last successful assessment is kept) rather than aborting the run.

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
