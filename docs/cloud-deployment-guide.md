# Cloud Deployment Guide

This guide provides step-by-step instructions for deploying Lab Monitor to the
Google Cloud Platform free tier using the new Cloud Run service mode.

## 1. Prerequisites

- Install the Google Cloud CLI (`gcloud`) and authenticate with your account.
- Install the Firebase CLI (for Authentication + optional Hosting).
- Ensure Docker is available locally for container builds (or rely on Cloud Build).

## 2. Project Setup

1. Create (or select) a GCP project:
   ```sh
   gcloud projects create lab-monitor-prod --set-as-default
   ```
   Replace `lab-monitor-prod` with your chosen project ID.
2. Link billing (required even for free-tier use).
3. Enable required APIs:
   ```sh
   gcloud services enable \
     run.googleapis.com \
     artifactregistry.googleapis.com \
     cloudbuild.googleapis.com \
     firestore.googleapis.com \
     cloudscheduler.googleapis.com \
     secretmanager.googleapis.com \
     iamcredentials.googleapis.com \
     firebase.googleapis.com
   ```
4. Initialize Firestore in **Datastore mode** (one-time, done in console or via `gcloud`):
   ```sh
   gcloud firestore databases create --region=us-central
   ```

## 3. Artifact Registry & CI/CD

1. Create a Docker repository:
   ```sh
   gcloud artifacts repositories create lab-monitor \
     --repository-format=DOCKER \
     --location=us \
     --description="Lab Monitor images"
   ```
2. Configure Cloud Build trigger (console or YAML) to build the Dockerfile and push to Artifact Registry on `main` merges.
3. (Optional) Use GitHub Actions if preferred; ensure the workflow runs `gcloud auth configure-docker` and `gcloud run deploy` with workload identity federation.

## 4. Firestore + Secrets Bootstrap

1. Populate initial configuration (convert your `config.yaml` to JSON payload and store via a helper script or `gcloud firestore` command). The config document lives at `config/current`:
   ```sh
   gcloud firestore documents create config/current \
     --fields="payload=$(cat config.json)",schema_version=1
   ```
   Prepare `config.json` by running `python -c 'import json,yaml,sys; print(json.dumps(yaml.safe_load(open("config.yaml"))))' > config.json`.
2. Store credentials in Secret Manager:
   ```sh
   gcloud secrets create labmonitor-openai-key --data-file=openai.key
   gcloud secrets create labmonitor-ses-access --data-file=aws_access_key
   gcloud secrets create labmonitor-ses-secret --data-file=aws_secret_key
   ```
   Later, wire these into Cloud Run environment variables using Secret Manager references.

## 5. Deploy Cloud Run Service

1. Build and push the container:
   ```sh
   gcloud builds submit --tag us-docker.pkg.dev/$(gcloud config get-value project)/lab-monitor/service:latest
   ```
2. Deploy to Cloud Run:
   ```sh
   gcloud run deploy lab-monitor \
     --image us-docker.pkg.dev/$(gcloud config get-value project)/lab-monitor/service:latest \
     --region=us-central1 \
     --platform=managed \
     --allow-unauthenticated=false \
     --set-env-vars GOOGLE_CLOUD_PROJECT=$(gcloud config get-value project) \
     --set-env-vars LABMONITOR_EMAIL_DRY_RUN=false \
     --set-env-vars LABMONITOR_RUN_TIMEOUT=4m \
     --set-secrets AWS_ACCESS_KEY_ID_SES=labmonitor-ses-access:latest \
     --set-secrets AWS_SECRET_ACCESS_KEY_SES=labmonitor-ses-secret:latest \
     --set-secrets OPENAI_API_KEY=labmonitor-openai-key:latest
   ```
   Adjust `--allow-unauthenticated` once authentication is configured (keep false for now).

## 6. Authentication

1. Create a Firebase project linked to the GCP project:
   ```sh
   firebase login
   firebase use $(gcloud config get-value project)
   firebase apps:create web lab-monitor-dashboard
   ```
2. Configure Firebase Authentication providers (Email/Password or Google Sign-In).
3. Update the Cloud Run service to require authentication by enabling Identity-Aware Proxy or by validating Firebase ID tokens server-side (see TODO section below).
4. Store admin user emails in Firestore (e.g., `users/{uid}` documents with `role=admin`).

## 7. Cloud Scheduler

1. Create a service account for Scheduler:
   ```sh
   gcloud iam service-accounts create labmonitor-scheduler \
     --display-name="Lab Monitor Scheduler"
   ```
2. Grant it the **Cloud Run Invoker** role:
   ```sh
   gcloud run services add-iam-policy-binding lab-monitor \
     --member=serviceAccount:labmonitor-scheduler@$(gcloud config get-value project).iam.gserviceaccount.com \
     --role=roles/run.invoker
   ```
3. Create the job that triggers `/api/run` daily (example 07:00 UTC):
   ```sh
   gcloud scheduler jobs create http labmonitor-daily \
     --schedule="0 7 * * *" \
     --uri="https://lab-monitor-<hash>-uc.a.run.app/api/run" \
     --http-method=POST \
     --oidc-service-account-email=labmonitor-scheduler@$(gcloud config get-value project).iam.gserviceaccount.com
   ```
   Replace the URL with the Cloud Run service URL.

## 8. Dashboard Hosting

1. Build the forthcoming dashboard front end (once implemented) and deploy to Firebase Hosting:
   ```sh
   firebase hosting:sites:create lab-monitor-dashboard
   firebase deploy --only hosting:lab-monitor-dashboard
   ```
2. Configure the SPA to call the Cloud Run API with Firebase ID tokens attached.

## 9. Monitoring & Alerts

1. Create log-based metrics for `state="failed"` run outcomes.
2. Configure Cloud Monitoring alerts (email/sms) when failures exceed thresholds.
3. Set a Cloud Storage bucket and schedule Firestore exports weekly (keep within free tier by pruning).

## 10. Local Testing with Emulators

1. Install the Firestore emulator:
   ```sh
   gcloud components install cloud-firestore-emulator
   ```
2. Run the server locally pointing at the emulator:
   ```sh
   export FIRESTORE_EMULATOR_HOST=localhost:8085
   export GOOGLE_CLOUD_PROJECT=fake-lab-monitor
   gcloud beta emulators firestore start --host-port=localhost:8085
   go run ./cmd/server -config-backend=firestore -report-backend=firestore -dry-run
   ```
   You will still need to seed config using the emulator admin UI or via REST requests.

## TODO / Manual Follow-ups

- Implement Firebase token verification middleware (replace the `NoopAuthenticator`).
- Wire OpenAI / SES credentials from Secret Manager into environment variables expected by the application.
- Build out the dashboard front end and connect it to the new API.
- Define IAM policies for admin vs viewer roles.
- Automate Firestore config seeding and backups (Terraform or scripts).

Track progress against these items as you migrate off the Raspberry Pi environment.

