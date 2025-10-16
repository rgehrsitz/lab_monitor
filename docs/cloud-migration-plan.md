# Lab Monitor Cloud Migration Plan

This document outlines the technical plan for moving the Raspberry Pi–hosted Lab Monitor
service to Google Cloud Platform (GCP) while adding remote configuration, scheduling control,
and a lightweight web dashboard. The target scale is fewer than a dozen trusted users and
strict adherence to always-free (or effectively free) GCP tiers.

## 1. Target Architecture

- **Runtime**: Cloud Run (fully managed). Containerized Go service exposed via HTTPS.
- **State & Config**: Firestore (Datastore mode) storing configuration, run history, and
  email profile metadata. Object retention for historical reports via Cloud Storage (optional).
- **Scheduling**: Cloud Scheduler calling authenticated Cloud Run endpoints for periodic runs.
- **Authentication**: Firebase Authentication (email+password or Google Sign-In), with custom
  claims for `admin` vs `viewer`. Cloud Run validates Firebase ID tokens.
- **Secrets**: Secret Manager storing API keys (ThingSpeak, OpenAI, AWS SES credentials, etc.).
- **Front End**: Single-page HTML dashboard served by Cloud Run (same binary) or Firebase Hosting.
- **Observability**: Cloud Logging, Cloud Monitoring alerting, optional Error Reporting.

## 2. Application Refactor Summary

1. **Configuration Provider Interface**
   - Introduce an interface (e.g. `ConfigStore`) that supports CRUD on schedule, channels, email profiles, style options.
   - Implement a Firestore-backed provider and keep a file-based provider for local development.
   - Add optimistic concurrency (etag/version field) to prevent clobbering concurrent updates.

2. **State Persistence Abstraction**
   - Replace file-based `state.Store` with interface `ReportStore` supporting:
     - `SaveReport(record)`
     - `LatestReport()`
     - `History(limit)`
   - Provide Firestore implementation (with TTL index for trimming) and keep filesystem implementation for offline testing.

3. **Service Runtime Changes**
   - Decouple `app.Service` from static config. Inject `ConfigProvider` so each run fetches fresh settings.
   - Expose service methods for:
     - `Run(ctx, now)` (unchanged signature)
     - `RunWithConfig(ctx, cfg, now)` (for manual runs/tests)
   - Emit structured run status for the HTTP layer to respond to dashboard clients.

4. **HTTP API Layer**
   - New `internal/server` package with handlers:
     - `GET /api/status` – current config snapshot, last run metadata, next scheduled run.
     - `POST /api/run` – immediate run trigger (admin only).
     - `GET /api/config` – read config (admin or viewer with limited fields).
     - `PUT /api/config` – update config (admin only).
     - `PATCH /api/config/emails` – manage email recipients/profiles.
     - `POST /api/schedule/test` – dry-run schedule validation.
   - Middleware validating Firebase ID tokens + role claims.
   - SSE or WebSocket endpoint (optional) streaming run events for live dashboard updates.

5. **Dashboard**
   - Minimal SPA (HTMX or React) served at `/` using the API.
   - Features: start/stop indicator, next run countdown, run history table, editable config forms, run-now button.
   - Authentication flow bootstraps Firebase Auth; token stored in memory and attached to API requests.

6. **Scheduler Integration**
   - Replace local cron with Cloud Scheduler hitting `POST /api/run` using service-account OIDC token.
   - Store schedule definition in Firestore. Cloud Scheduler job is updated when schedule changes
     (via post-commit trigger or manual admin UI calling a dedicated endpoint).

7. **Email Delivery**
   - Continue using AWS SES (current implementation) or switch to Mailjet/SendGrid.
   - Move credentials to Secret Manager and inject via environment variables.

## 3. Repository Changes (Planned)

- Add `cmd/server/main.go` entrypoint launching HTTP server + background runner.
- Introduce `internal/server` (HTTP API), `internal/configstore`, `internal/reportstore`.
- Refactor `app.Service` to operate with dynamic config + Firestore store.
- Add Dockerfile, `.dockerignore`, and local `Makefile` targets for container builds.
- Include integration tests leveraging emulator (Firestore emulator recommended).
- Create Firebase web app scaffolding under `web/` with minimal dashboard.
- Update README with local dev instructions (running emulators, API usage).

## 4. GCP Provisioning Checklist

1. Create dedicated GCP project.
2. Enable APIs: Cloud Run, Firestore, Cloud Build, Artifact Registry, Cloud Scheduler, Secret Manager, IAM, Firebase.
3. Initialize Firestore (Datastore mode).
4. Create Artifact Registry repo (`us-docker.pkg.dev/<project>/lab-monitor/service`).
5. Provision Firebase project + web app for auth.
6. Grant Cloud Run Service Account access to Firestore, Secret Manager.
7. Configure Cloud Scheduler job posting to Cloud Run endpoint with OIDC token.
8. Set up Cloud Build trigger or GitHub Actions for CI/CD.
9. Configure log-based metrics + alert policies.

## 5. Migration Steps

1. Implement code refactor locally; confirm file-backed mode still works on Raspberry Pi.
2. Build container image locally, run with Firestore emulator to validate new flows.
3. Deploy to Cloud Run (staging) with limited traffic; run manual tests via API.
4. Configure dashboard + Firebase auth; grant admin claims to trusted users.
5. Validate Cloud Scheduler job executes runs; monitor logs.
6. Decommission Raspberry Pi cron after confidence window.

## 6. Cost Controls

- Cloud Run free tier covers up to 2M requests/month; set minimal CPU & memory (e.g. 0.25 vCPU, 512 MiB).
- Firestore reads/writes well within 50k/day free quota given <12 users.
- Cloud Scheduler: first 3 jobs free.
- Firebase Auth & Hosting: free for this scale.
- Use Cloud Monitoring budgets/alerts to watch for cost anomalies.

## 7. Open Questions

- Retain AWS SES or migrate to a Google-native email solution?
- Do we need background jobs beyond daily run (e.g., hourly health checks)?
- Would WebSocket updates materially benefit dashboard users over periodic polling?

These will be addressed during implementation.

