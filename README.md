# job-app-dispatch

**Status: Archived. Not actively maintained. Does not work reliably end-to-end.**

A Go daemon + Python subprocess job-search automation system. Scrapes
Greenhouse/Ashby/Lever ATS APIs and free job boards (RemoteOK, Arbeitnow),
scores results, generates cover letters via the Anthropic API, and presents
a web UI for human review before submission.

## Why it's archived

I built this as a portfolio piece that would double as a working job-search
tool. On first-run testing it became clear the architecture had choices that
made it impractical to actually use, and that a more complete open-source
alternative ([santifer/career-ops](https://github.com/santifer/career-ops))
already covered the ground I cared about — with features I hadn't built yet.
Rather than spend another several days patching the gaps, I chose to stop
and adopt/customize career-ops instead.

## Known flaws and shortcomings

- **First-run cost cliff.** The pipeline calls the Anthropic API once per
  scraped job to generate a match rationale, with no cap and no score
  threshold. A first run against ~20 seeded portals yields ~250+ jobs,
  which meant ~250+ API calls and $1-3 of unexpected spend before anything
  even appeared in the UI. Steady-state (5-20 new jobs/day) would have been
  fine; the batch-from-cold-start case was not.

- **No PDF resume tailoring.** The scorer produces per-job keyword diffs
  and stores them in the DB, but there is no PDF-generation step — a major
  missing feature for ATS-optimized applications.

- **No batch/parallel AI evaluation.** Every rationale call is serial,
  compounding the cost-cliff problem. career-ops solves this with
  Claude Code sub-agents; this project doesn't.

- **Scorer filters are too lenient by default.** Location, salary-floor,
  and archetype filters exist but let too many jobs through to the AI
  step. The right fix is either a hard top-N cap or a score threshold
  gate before the API call — neither is implemented.

- **Skills extraction is a stub.** `parse_resume.py` returns an empty list
  for `skills` — the TODO is marked in the code.

- **Setup papercuts I did patch but shouldn't have existed:**
  - `godotenv.Load()` was called with no path, so `.env` was only picked
    up when run from the project root, not from `core/` where `go run`
    expects to live.
  - The fsnotify resume watcher only fired on file *changes* and had no
    initial scan of existing files, so resumes dropped in pre-startup
    were never parsed.

- **Discord connection drops silently** in some network conditions; no
  reconnect logic.

## What works

- ATS scrapers (Greenhouse, Ashby, Lever) via portals.yaml
- RemoteOK and Arbeitnow scrapers
- Single-URL fast-track ingest (`POST /jobs/ingest`)
- Resume parsing (PDF and DOCX), filename-based role classification
- Scoring with archetype detection and a legitimacy tier
- Web UI: digest, approve/reject, cover-letter editor, history page
- Discord notifications (optional; app runs fine without credentials)
- Scheduled cron runs

## If you want to look at this anyway

See `SETUP.md` for the original setup guide and `CODEBASE.md` for an
architecture overview. The code compiles and runs; the above caveats
are the reasons I don't recommend it for actual use.

## License

No license specified — treat as all-rights-reserved unless I add one later.
