# Setup Guide

This is a one-time walkthrough. Once done, the app runs on its own.

---

## What you need before starting

- **Go 1.22+** — `brew install go`
- **Python 3.11+** — already on your Mac via Homebrew
- **An Anthropic API key** — for cover letter generation
- **A Discord server** you control — for job notifications
- **~15 minutes**

---

## Step 1 — Python dependencies

Install the packages the scraper and resume parser need:

```bash
pip3 install --break-system-packages -r requirements.txt
```

Verify it worked:

```bash
python3 -c "import requests, pydantic, yaml; print('ok')"
```

---

## Step 2 — Anthropic API key

1. Go to [console.anthropic.com](https://console.anthropic.com)
2. Sign in → **API Keys** → **Create Key**
3. Copy the key (starts with `sk-ant-`)

You'll add it to `.env` in Step 4.

---

## Step 3 — Discord bot setup

The app sends you a message when new jobs are ready and when an application is submitted or fails. You need a bot in a server you own (even a private one just for yourself).

**Create the bot:**

1. Go to [discord.com/developers/applications](https://discord.com/developers/applications)
2. **New Application** → give it a name (e.g. "job-dispatch")
3. Click **Bot** in the left sidebar
4. Click **Reset Token** → copy the token (you won't see it again)
5. Scroll down to **Privileged Gateway Intents** → enable **Message Content Intent**
6. Save changes

**Invite it to your server:**

1. Click **OAuth2** → **URL Generator** in the left sidebar
2. Under Scopes check: `bot`
3. Under Bot Permissions check: `Send Messages`, `View Channels`
4. Copy the generated URL → open it in your browser → select your server → Authorize

**Get the channel ID:**

1. In Discord, open User Settings → **Advanced** → enable **Developer Mode**
2. Right-click the channel you want notifications in → **Copy Channel ID**

---

## Step 4 — Create your `.env` file

```bash
cp .env.example .env
```

Open `.env` and fill in:

```
ANTHROPIC_API_KEY=sk-ant-your-key-here
DISCORD_TOKEN=your-bot-token-here
DISCORD_CHANNEL_ID=your-channel-id-here
```

Never commit this file. It's already in `.gitignore`.

---

## Step 5 — Configure `config.yaml`

```bash
cp config.example.yaml config.yaml
```

Open `config.yaml` and fill in the sections that matter to you:

```yaml
schedule:
  cron: "0 7 * * *"   # runs daily at 7am — adjust as you like

locations:
  onsite_allowed:
    - "Albuquerque, NM"   # on-site/hybrid jobs here are shown, not penalised
    - "Santa Fe, NM"       # add or remove cities as needed

salary:
  floor_hourly: 20.0    # jobs with salary listed BELOW this are dropped

profile:
  linkedin_url: "https://linkedin.com/in/your-profile"
  github_url: "https://github.com/your-username"
```

The `targets.role_keywords` section already has sensible defaults for dev and IT roles. Adjust them if the app is surfacing wrong job types.

---

## Step 6 — Set up your company watchlist

```bash
cp portals.example.yaml portals.yaml
```

Open `portals.yaml`. It comes pre-seeded with ~20 companies (Cloudflare, Stripe, Linear, Vercel, etc.). Edit it to reflect companies you actually want to track.

To add a company you're interested in:

1. Find their careers page
2. Check the URL — if it contains `greenhouse.io`, `ashby.com`, or `lever.co`, you're good
3. The slug is in the URL: `boards.greenhouse.io/cloudflare` → slug is `cloudflare`
4. Add an entry:
   ```yaml
   - name: "Company Name"
     ats: "greenhouse"   # or ashby / lever
     slug: "their-slug"
   ```

If a company uses a different system (Workday, Lever, BambooHR), it can be added later. For now, focus on Greenhouse/Ashby/Lever companies — they cover a large chunk of tech.

---

## Step 7 — Add your resume files

Drop your resume files into `materials/resumes/`:

```
materials/resumes/resume_dev.pdf    ← dev roles (fullstack, backend, etc.)
materials/resumes/resume_it.pdf     ← IT roles (helpdesk, sysadmin, etc.)
```

Filenames should contain `dev` or `it` so the parser classifies them correctly. The app watches this folder — if you update a file while the app is running, it re-parses automatically.

If you only have one resume, put it in both paths or just use the dev path and update `config.yaml` accordingly.

---

## Step 8 — (Optional) Add cover letter samples

Drop 2–3 past cover letters into `materials/cover_letter_samples/` as `.txt` files. The AI uses these as a tone and style reference — it won't copy them, just match your voice. If you skip this, the AI uses a sensible default style.

---

## Step 9 — Go dependencies

```bash
cd core && go mod tidy
```

---

## Step 10 — Run the app

```bash
cd core && go run cmd/main.go
```

You should see:

```
main: web UI at http://localhost:8080
main: discord: connected
main: watcher: watching materials/resumes
```

The first scrape runs on your configured schedule. To trigger one immediately without waiting, there will be a "Run Now" button in the web UI — or just restart the app after a schedule change.

---

## What happens after setup

1. At the scheduled time, the app scrapes your `portals.yaml` companies
2. Jobs are scored and filtered
3. Discord pings you with a count and a link
4. You open `localhost:8080`, review the cards, approve or reject
5. Approved jobs get a cover letter generated — you review and edit it
6. You click **Submit Application** → the job URL opens, you apply with the generated letter

That's the full loop. You interact with it for ~10–20 minutes per batch, the app handles the rest.

---

## Keeping the app running persistently

If you want it to run without needing a terminal open, use a LaunchAgent (macOS background service):

```bash
# Coming soon — will be added to the repo
```

For now, run it in a terminal tab and leave it open, or use a tool like `screen` or `tmux`:

```bash
tmux new -s job-dispatch
cd /path/to/job-app-dispatch/core && go run cmd/main.go
# Ctrl+B then D to detach — it keeps running
```

---

## Troubleshooting

**"portals.yaml not found"** — run `cp portals.example.yaml portals.yaml` from the repo root

**"no module named requests"** — run `pip3 install --break-system-packages -r requirements.txt`

**Discord not connecting** — double-check `DISCORD_TOKEN` in `.env` and that Message Content Intent is enabled in the developer portal

**No jobs appearing** — check the terminal for scraper errors; verify your `portals.yaml` slugs by opening the ATS API URL directly in a browser (e.g. `https://boards-api.greenhouse.io/v1/boards/cloudflare/jobs`)
