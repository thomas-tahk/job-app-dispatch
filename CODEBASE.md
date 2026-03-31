# job-app-dispatch — Codebase Reference

> Living document. Updated with every meaningful code change.
> Last updated: scrape pipeline, submission pipeline, web server wiring.

---

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Repository Layout](#repository-layout)
3. [Go Core (`core/`)](#go-core-core)
   - [Entry Point](#entry-point-corecmdmaingo)
   - [Models](#models-coreinternalmodelsmodelgo)
   - [Database](#database-coreinternaldbdbgo)
   - [Connector Interface & Runner](#connector-interface--runner-coreinternalconnector)
   - [Pipeline — Scrape](#pipeline-scrape-coreinternalpipelinepipelinego)
   - [Pipeline — Submission](#pipeline-submission-coreinternalpipelinesubmissiongo)
   - [Pipeline — Style Samples](#pipeline-style-samples-coreinternalpipelinesamplespy)
   - [Scorer](#scorer-coreinternalscorerscoreergo)
   - [AI Client](#ai-client-coreinternalaiaigo)
   - [Scheduler](#scheduler-coreinternalschedulerschedulergo)
   - [Discord Bot](#discord-bot-coreinternaldiscorddiscordgo)
   - [Resume Watcher](#resume-watcher-coreinternalwatcherresumesgo)
   - [Web Server](#web-server-coreinternalwebservergo)
4. [Python Connectors (`connectors/`)](#python-connectors-connectors)
   - [Shared Models](#shared-models-connectorssharedmodelspy)
   - [Shared Browser Helpers](#shared-browser-helpers-connectorsshaaredbrowserpy)
   - [LinkedIn Scraper](#linkedin-scraper-connectorslinkedinscrraperpy)
   - [LinkedIn Submitter](#linkedin-submitter-connectorslinkedinsubmitterpy)
   - [Indeed Scraper](#indeed-scraper-connectorsindeedscrraperpy)
   - [Indeed Submitter](#indeed-submitter-connectorsindeedsubmitterpy)
5. [Resume Parser (`scripts/`)](#resume-parser-scripts)
6. [Configuration (`config.yaml`)](#configuration-configyaml)
7. [Data Flow: End to End](#data-flow-end-to-end)
8. [Adding a New Connector](#adding-a-new-connector)
9. [Setup & Running](#setup--running)
10. [Environment Variables](#environment-variables)
11. [Implementation Status](#implementation-status)

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                        Go Core                              │
│                                                             │
│  Scheduler ──► Pipeline (scrape → score → store → notify)  │
│                    │                                        │
│                    ▼                                        │
│  connector.Runner ──► Python subprocess (JSON stdout)       │
│                                                             │
│  Web UI (localhost:8080) ◄── user reviews, edits, approves  │
│  Discord Bot ──► channel ping (new jobs / submission result) │
│  Resume Watcher ──► Python parser subprocess on file change  │
└─────────────────────────────────────────────────────────────┘
         │ subprocess JSON                  │ subprocess JSON
         ▼                                  ▼
┌─────────────────┐               ┌──────────────────┐
│ Python Scraper  │               │ Python Submitter  │
│ (Playwright)    │               │ (Playwright)      │
└─────────────────┘               └──────────────────┘
```

**Key design decisions:**

- **Go/Python split**: Go handles all orchestration, persistence, UI, and AI calls. Python handles browser automation exclusively. This puts each language where its ecosystem is strongest.
- **Subprocess JSON interface**: Go communicates with Python via stdin/stdout JSON. This keeps connectors language-agnostic and independently testable (`python connectors/linkedin/scraper.py --config config.yaml` runs fine on its own).
- **Local web UI for interaction**: All approve/reject/cover-letter-editing happens in a browser at `localhost:8080`. Discord sends only fire-and-forget notifications.
- **Optimistic default**: Jobs are skipped only for hard violations (salary missing, salary below floor, no healthcare info, clear role mismatch with very low score). Seniority is never a hard filter.

---

## Repository Layout

```
job-app-dispatch/
├── core/                          # Go application
│   ├── cmd/
│   │   └── main.go                # Entry point
│   ├── internal/
│   │   ├── models/models.go       # DB models: Job, Application, Resume
│   │   ├── db/db.go               # SQLite open + auto-migrate
│   │   ├── connector/
│   │   │   ├── connector.go       # Scraper + Submitter interfaces; shared JSON types
│   │   │   └── runner.go          # PythonScraper + PythonSubmitter implementations
│   │   ├── scorer/scorer.go       # Match scoring logic
│   │   ├── ai/ai.go               # Claude API: cover letters + match rationale
│   │   ├── scheduler/scheduler.go # Cron scheduler wrapper
│   │   ├── discord/discord.go     # Discord notification bot
│   │   ├── watcher/resumes.go     # fsnotify watcher for materials/resumes/
│   │   └── web/
│   │       ├── server.go          # Chi HTTP server + all route handlers
│   │       └── templates/
│   │           ├── digest.html    # Job review page
│   │           └── cover_edit.html# Cover letter editor
│   └── go.mod
│
├── connectors/                    # Python browser automation
│   ├── shared/
│   │   ├── models.py              # Pydantic ScrapedJob, SubmitRequest, SubmitResult
│   │   └── browser.py             # Playwright context factory + human_delay
│   ├── linkedin/
│   │   ├── scraper.py             # LinkedIn search scraper
│   │   └── submitter.py           # LinkedIn Easy Apply submitter
│   └── indeed/
│       ├── scraper.py             # Indeed search scraper
│       └── submitter.py           # Indeed Apply submitter
│
├── scripts/
│   └── parse_resume.py            # Parses .docx/.pdf → structured JSON
│
├── materials/                     # gitignored personal files
│   ├── resumes/                   # Drop resume files here (.pdf + .docx)
│   └── cover_letter_samples/      # Past cover letters for AI style reference
│
├── data/
│   └── jobs.db                    # SQLite database (gitignored)
│
├── config.yaml                    # Non-secret settings
├── .env                           # Secrets (gitignored — copy from .env.example)
├── .env.example                   # Template for required env vars
├── requirements.txt               # Python dependencies
└── CODEBASE.md                    # This file
```

---

## Go Core (`core/`)

### Entry Point (`core/cmd/main.go`)

Bootstraps and wires together all components:

1. Loads `.env` via `godotenv`, then reads `config.yaml`.
2. Opens the SQLite database.
3. Initialises the Discord bot and opens the websocket connection.
4. Creates the AI client with the Anthropic API key.
5. Registers the scrape pipeline on the configured cron schedule.
6. Starts the resume watcher (background goroutine via `fsnotify`).
7. Starts the web UI server (background goroutine).
8. Blocks on `SIGINT`/`SIGTERM` for clean shutdown.

Key functions:
- `parseAndStoreResume(db, path)` — calls `scripts/parse_resume.py`, unmarshals the JSON, upserts into the `resumes` table.
- `runScrapeAndScore(db, aiClient, bot, cfg)` — the main pipeline function, currently a stub with documented TODOs.

---

### Models (`core/internal/models/models.go`)

Three GORM models stored in SQLite:

| Model | Purpose |
|---|---|
| `Job` | One row per unique listing. `ExternalID + Source` pair is unique — deduplication key. `Status` tracks the full lifecycle. |
| `Application` | Created when a job is approved. Holds the final cover letter and submission result. One-to-one with `Job`. |
| `Resume` | One row per resume file. Populated/updated by the resume watcher + parser. |

**`JobStatus` values:**
- `new` — freshly scraped, awaiting review
- `approved` — user approved in UI, queued for cover letter generation + submission
- `rejected` — user rejected; never shown again
- `submitted` — successfully submitted
- `failed` — automation failed; manual URL sent to Discord
- `manual` — flagged for manual application from the start

**`RoleType` values:** `dev` | `it` — determines which resume PDF is attached.

---

### Database (`core/internal/db/db.go`)

Opens a SQLite file at the given path using GORM. Calls `AutoMigrate` on startup, so schema is always current without manual migrations for early development.

---

### Connector Interface & Runner (`core/internal/connector/`)

**`connector.go`** — pure interfaces and shared JSON types:

```go
type Scraper interface {
    Source() string
    Scrape(ctx context.Context) ([]ScrapedJob, error)
}
type Submitter interface {
    Source() string
    Submit(ctx context.Context, req SubmitRequest) (SubmitResult, error)
}
```

`ScrapedJob`, `SubmitRequest`, and `SubmitResult` are the JSON contract between Go and Python. Field names are snake_case to match Pydantic models on the Python side.

**`runner.go`** — subprocess implementations:

- `PythonScraper.Scrape()` — runs `python <scriptPath> --config <configPath>`, reads stdout, unmarshals JSON array.
- `PythonSubmitter.Submit()` — marshals `SubmitRequest` to JSON, writes to stdin of `python <scriptPath>`, reads `SubmitResult` from stdout.

Both capture stderr from the Python process and include it in the error message for easier debugging.

---

### Pipeline — Scrape (`core/internal/pipeline/pipeline.go`)

`Runner` owns all dependencies (DB, scrapers, submitters, AI client, Discord bot, config) and exposes two entry points used by the rest of the app.

`Runner.Run(ctx)` is the main scrape cycle, triggered by the scheduler:

1. Calls every registered `Scraper` subprocess — failures are logged and skipped, not fatal.
2. **Deduplicates** against the DB using `ExternalID + Source`. Previously seen jobs are silently dropped.
3. Loads the best-matching `Resume` record from the DB for each job (dev vs IT, based on title/description keywords).
4. **Hard-filters** via `scorer.Score` — skips jobs with missing salary, below-floor salary, or no benefits info.
5. **Scores** remaining jobs across role alignment, salary, location, skills, and seniority.
6. **Location tagging**: on-site jobs outside the configured allowed cities get `RequiresRelocation = true` but are still stored.
7. Calls `AI.GenerateMatchRationale` for each kept job — a one-liner digest blurb. Errors here are non-fatal.
8. Persists new `Job` rows and Discord-notifies with the count.

Helper functions (unexported): `deduplicate`, `pickResume`, `isAllowedLocation`.

---

### Pipeline — Submission (`core/internal/pipeline/submission.go`)

Two methods on `Runner`:

**`ProcessApproval(ctx, jobID) error`** — called by the web handler when the user clicks Approve:
1. Loads job + matching resume from DB.
2. Loads style samples from `materials/cover_letter_samples/`.
3. Calls `AI.GenerateCoverLetter` with job details, resume text, samples, and profile links.
4. Creates an `Application` record with the generated cover letter and the appropriate resume PDF path.
5. Returns — submission is NOT triggered yet. User reviews the letter first.

**`Submit(ctx, jobID)`** — called (in a goroutine) when the user clicks Submit Application:
1. Loads job + application from DB.
2. Looks up the registered `Submitter` for the job's source.
3. Calls the submitter subprocess with `SubmitRequest` JSON via stdin.
4. On success: updates `Application.SubmittedAt`, sets `Job.Status = submitted`, Discord-notifies.
5. On failure: sets `Job.Status = failed`, saves failure reason, Discord-notifies with manual apply URL.

`jobToScrapedJob` maps a stored `models.Job` back to `connector.ScrapedJob` for the submitter interface.

---

### Pipeline — Style Samples (`core/internal/pipeline/samples.go`)

`LoadStyleSamples(dir)` reads all `.txt` and `.md` files from `materials/cover_letter_samples/` and returns their text. These are passed to `GenerateCoverLetter` as tone/voice reference. PDF cover letter samples are not parsed here; save them as `.txt` or `.md`.

---

### Scorer (`core/internal/scorer/scorer.go`)

`Score(job, resume, cfg)` returns a `Result` with a 0.0–1.0 score and a `ShouldSkip` flag.

**Hard filters (cause immediate skip):**
- Salary not listed (`salary_min == 0 && salary_max == 0`)
- Salary below configured floor
- No benefits information at all

**Scoring dimensions** (weighted sum):

| Dimension | Weight | Notes |
|---|---|---|
| Role alignment | 30% | Full score if role type matches resume; 0.3 if not |
| Salary | 20% | Scaled by how far above the floor |
| Location | 25% | Remote = 1.0; local on-site = 0.8; relocation = 0.4 |
| Skills | 20% | Keyword match of resume skills vs. description (TODO: fully implement) |
| Seniority | 5% | Soft signal only; junior = 1.0, senior = 0.6, mid = 0.8 |

**Mismatch skip threshold:** Score < 0.15 AND wrong role type → skipped. Otherwise always surfaces.

---

### AI Client (`core/internal/ai/ai.go`)

Wraps `anthropic-sdk-go` v1.28.0.

- `New(apiKey)` — creates a client using `option.WithAPIKey`. The `Client` struct holds an `anthropic.Client` value (not pointer — this is how the v1.x SDK works).
- `GenerateCoverLetter(ctx, req)` — builds a prompt including style sample letters (tone/voice reference only), the job posting, resume text, and LinkedIn/GitHub URLs. Uses `ModelClaudeSonnet4_6`, max 1024 tokens.
- `GenerateMatchRationale(ctx, ...)` — one sentence (≤20 words) explaining the match. Uses `ModelClaudeSonnet4_6`, max 100 tokens.

**SDK API shape (v1.28.0):** `MessageNewParams` takes plain Go values — no `F()` wrapper needed. Response content is accessed via `msg.Content[0].Text` (field on `ContentBlockUnion`). `WithAPIKey` lives in the `option` subpackage.

---

### Scheduler (`core/internal/scheduler/scheduler.go`)

Thin wrapper around `robfig/cron/v3`. `AddJob(name, schedule, fn)` registers a named function on a standard cron expression. Schedule is loaded from `config.yaml` at startup — change it there and restart.

---

### Discord Bot (`core/internal/discord/discord.go`)

Uses `bwmarrin/discordgo` to send messages to a single channel. The bot does **not** handle slash commands or interactive buttons — all interactivity is in the web UI. Discord is notification-only.

Three notification types:
- `NotifyDigestReady(count, webAddr)` — "N new jobs ready → http://localhost:8080"
- `NotifySubmissionSuccess(job)` — "Applied to X at Y"
- `NotifySubmissionFailed(job, manualURL)` — "Submission failed for X — apply manually: <url>"

---

### Resume Watcher (`core/internal/watcher/resumes.go`)

Uses `fsnotify` to watch `materials/resumes/` for `Create` or `Write` events on `.pdf` or `.docx` files. When triggered, calls the `onChanged` callback (defined in `main.go`) which shells out to `scripts/parse_resume.py` and upserts the result.

Runs in a background goroutine; stops cleanly when the context is cancelled.

---

### Web Server (`core/internal/web/server.go`)

Chi HTTP router. Templates are embedded at compile time via `//go:embed templates/*`.

| Route | Handler | Description |
|---|---|---|
| `GET /` | `handleDigest` | Lists all `new` jobs sorted by score descending |
| `POST /jobs/{id}/approve` | `handleApprove` | Calls `onApprove` (generates cover letter, creates Application), redirects to cover editor |
| `POST /jobs/{id}/reject` | `handleReject` | Sets status → `rejected` |
| `GET /jobs/{id}/cover` | `handleCoverView` | Renders cover letter editor |
| `POST /jobs/{id}/cover` | `handleCoverSave` | Saves edited cover letter, stays on editor page |
| `POST /jobs/{id}/submit` | `handleSubmit` | Saves cover letter, fires `onSubmit` in a goroutine, redirects to digest |

`New()` accepts `onApprove` and `onSubmit` callbacks (wired to `pipeline.Runner` methods in `main.go`), keeping the web package free of pipeline imports. A `pct` template function converts 0.0–1.0 scores to 0–100 integers for display.

**Templates:**
- `digest.html` — job card list with Approve / Reject buttons and a link to the cover letter editor. Shows `[RELOCATION REQUIRED]` tag where applicable.
- `cover_edit.html` — textarea pre-filled with the AI-generated cover letter; saves on submit.

---

## Python Connectors (`connectors/`)

### Shared Models (`connectors/shared/models.py`)

Pydantic v2 models that mirror the Go connector types exactly:
- `ScrapedJob` — output of a scraper
- `SubmitRequest` — input to a submitter
- `SubmitResult` — output of a submitter

All scrapers and submitters import from here, ensuring the JSON contract stays consistent.

---

### Shared Browser Helpers (`connectors/shared/browser.py`)

- `get_browser_context(playwright, headless)` — launches Chromium with a realistic `User-Agent`, viewport, locale, and timezone. Masks the `navigator.webdriver` flag.
- `human_delay(min_ms, max_ms)` — random sleep between interactions to mimic human pacing.
- `random_mouse_move(page)` — moves mouse to a random position; helps with some anti-bot checks.

---

### LinkedIn Scraper (`connectors/linkedin/scraper.py`)

**Input:** `--config <path>` (yaml)
**Output:** JSON array of `ScrapedJob` to stdout

Reads target role keywords and locations from config, constructs LinkedIn search URLs, and uses Playwright to navigate and extract job cards.

**TODO:** Implement search navigation, card parsing, pagination, session cookie persistence.

---

### LinkedIn Submitter (`connectors/linkedin/submitter.py`)

**Input:** `SubmitRequest` JSON via stdin
**Output:** `SubmitResult` JSON to stdout

For Easy Apply jobs: navigates the LinkedIn Easy Apply modal. Falls back to `manual_url` if the form has multi-step questions beyond basic info.
For external jobs: immediately returns `manual_url`.

**TODO:** Implement Easy Apply form automation.

---

### Indeed Scraper (`connectors/indeed/scraper.py`)

**Input:** `--config <path>` (yaml)
**Output:** JSON array of `ScrapedJob` to stdout

Same pattern as LinkedIn scraper. Indeed uses heavy JS rendering — Playwright waits for job card list before parsing. Uses Indeed's `jobKeys` parameter as `external_id`.

**TODO:** Implement search navigation, card parsing, pagination.

---

### Indeed Submitter (`connectors/indeed/submitter.py`)

**Input:** `SubmitRequest` JSON via stdin
**Output:** `SubmitResult` JSON to stdout

Same pattern as LinkedIn submitter, targeting Indeed's "Apply on Indeed" flow.

**TODO:** Implement Indeed Apply form automation.

---

## Resume Parser (`scripts/`)

### `scripts/parse_resume.py`

**Input:** `<filepath>` argument (`.pdf` or `.docx`)
**Output:** JSON object to stdout:
```json
{
  "filename": "resume_dev.pdf",
  "role_type": "dev",
  "parsed_text": "...",
  "skills": ["Python", "Go", "React"],
  "titles": ["Software Engineer", "Frontend Developer"]
}
```

- `.docx` parsed with `python-docx` (paragraph text, preserves structure)
- `.pdf` parsed with `pdfplumber` (page text extraction)
- `classify_role_type` — uses filename first (`resume_dev` → `dev`), falls back to keyword frequency in text
- `extract_skills` and `extract_titles` — currently stubs (TODO)

Called automatically by `main.go` via `parseAndStoreResume()` whenever the watcher fires.

---

## Configuration (`config.yaml`)

All non-secret configuration lives here. Restart the app to pick up changes.

| Key | Type | Description |
|---|---|---|
| `schedule.cron` | string | Standard cron expression for the scrape pipeline |
| `locations.onsite_allowed` | list | Cities where on-site/hybrid jobs are surfaced |
| `salary.floor_hourly` | float | Minimum acceptable hourly rate; listings below this are skipped |
| `targets.role_keywords.dev` | list | Title/description keywords that classify a job as dev |
| `targets.role_keywords.it` | list | Title/description keywords that classify a job as IT |
| `web.addr` | string | Address for the local web UI (default `:8080`) |
| `resumes.dev` | string | Path to the dev resume PDF for submissions |
| `resumes.it` | string | Path to the IT resume PDF for submissions |
| `profile.linkedin_url` | string | Your LinkedIn URL — injected into cover letters |
| `profile.github_url` | string | Your GitHub URL — injected into cover letters |

---

## Data Flow: End to End

```
[Cron fires]
     │
     ▼
runScrapeAndScore()
     │
     ├─► PythonScraper (LinkedIn) subprocess → []ScrapedJob
     ├─► PythonScraper (Indeed)   subprocess → []ScrapedJob
     │
     ▼
Deduplicate against DB (ExternalID + Source)
     │
     ▼
Hard filter: salary, healthcare, location rules
     │
     ▼
Score each job → MatchScore + RoleType
     │
     ▼
AI: GenerateMatchRationale (short digest blurb)
     │
     ▼
INSERT new jobs into DB (status = "new")
     │
     ▼
Discord: NotifyDigestReady(count, webAddr)

[User opens localhost:8080]
     │
     ├─► Reject → status = "rejected" (never shown again)
     │
     └─► Approve
              │
              ▼
         AI: GenerateCoverLetter
              │
              ▼
         INSERT Application (status = "approved")
              │
              ▼
         [User optionally edits cover letter at /jobs/{id}/cover]
              │
              ▼
         PythonSubmitter subprocess → SubmitResult
              │
              ├─ success → status = "submitted" → Discord: NotifySubmissionSuccess
              └─ failure → status = "failed"    → Discord: NotifySubmissionFailed (+ manual URL)
```

---

## Adding a New Connector

1. Create `connectors/<platform>/scraper.py` following the LinkedIn/Indeed pattern:
   - Accept `--config <path>` argument
   - Output `[ScrapedJob, ...]` JSON array to stdout
   - Import `ScrapedJob` from `connectors/shared/models.py`

2. Create `connectors/<platform>/submitter.py`:
   - Read `SubmitRequest` JSON from stdin
   - Output `SubmitResult` JSON to stdout

3. In `core/cmd/main.go`, register the new connector's scraper and submitter using `connector.NewPythonScraper` and `connector.NewPythonSubmitter`. No other Go files need changes.

That's it. The connector interface is language-agnostic — the new scripts just need to speak the JSON contract.

---

## Setup & Running

**Prerequisites:** Go 1.22+, Python 3.11+

```bash
# 1. Python dependencies
pip install -r requirements.txt
playwright install chromium

# 2. Go dependencies (generates go.sum)
cd core && go mod tidy

# 3. Copy and fill in secrets
cp .env.example .env
# edit .env with your API keys

# 4. Fill in config.yaml
#    - Add your LinkedIn and GitHub URLs
#    - Adjust schedule cron if desired

# 5. Drop your resume files into materials/resumes/
#    (watcher will parse them automatically on first run)

# 6. Run
cd core && go run cmd/main.go
# Web UI: http://localhost:8080
```

---

## Environment Variables

| Variable | Required | Description |
|---|---|---|
| `ANTHROPIC_API_KEY` | Yes | Anthropic API key for cover letter generation |
| `DISCORD_TOKEN` | Yes | Discord bot token |
| `DISCORD_CHANNEL_ID` | Yes | Target channel ID for notifications |

---

## Implementation Status

| Component | Status |
|---|---|
| Project scaffold & interfaces | ✅ Complete |
| Go models + DB | ✅ Complete |
| Connector subprocess runner | ✅ Complete |
| Scorer (hard filters + weighted score) | ✅ Complete — skills matching TODO |
| AI client (cover letter + rationale) | ✅ Complete — verified against SDK v1.28.0 |
| Discord notifications | ✅ Complete |
| Resume watcher + parser | ✅ Complete — skills/title extraction TODO |
| Scrape pipeline | ✅ Complete |
| Submission pipeline (approval → cover letter → submit) | ✅ Complete |
| Web UI (digest + cover editor + submit flow) | ✅ Complete |
| Scheduler | ✅ Complete |
| Entire module compiles (`go build ./...`) | ✅ Verified |
| LinkedIn scraper | ⬜ TODO |
| LinkedIn submitter | ⬜ TODO |
| Indeed scraper | ⬜ TODO |
| Indeed submitter | ⬜ TODO |
| Skills extraction in resume parser | ⬜ TODO |
| Salary parsing in scrapers | ⬜ TODO |
