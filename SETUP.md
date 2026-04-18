# Setup Guide

Full start-to-finish checklist. Do these in order.

---

## 1. Anthropic API key

1. Go to [console.anthropic.com](https://console.anthropic.com) → sign in
2. **API Keys** → **Create Key** → copy it (starts with `sk-ant-`)

---

## 2. Discord bot

1. Go to [discord.com/developers/applications](https://discord.com/developers/applications)
2. **New Application** → name it anything (e.g. "job-dispatch")
3. Left sidebar → **Bot**
4. **Reset Token** → copy the token
5. Scroll down → **Privileged Gateway Intents** → enable **Message Content Intent** → Save
6. Left sidebar → **OAuth2** → **URL Generator**
   - Scopes: check `bot`
   - Bot Permissions: check `Send Messages` and `View Channels`
   - Copy the generated URL → open it in your browser → select your server → Authorize
7. In Discord: **User Settings** → **Advanced** → enable **Developer Mode**
8. Right-click the channel you want notifications in → **Copy Channel ID**

> Discord is optional. If you leave the credentials blank in `.env` the app starts fine
> and logs notifications to the terminal instead.

---

## 3. Create your `.env` file

```bash
cp .env.example .env
```

Open `.env` and fill it in:

```
ANTHROPIC_API_KEY=sk-ant-your-key-here
DISCORD_TOKEN=your-bot-token-here
DISCORD_CHANNEL_ID=your-channel-id-here
```

---

## 4. Create your `config.yaml`

```bash
cp config.example.yaml config.yaml
```

Open it and update these sections:

```yaml
locations:
  onsite_allowed:
    - "Albuquerque, NM"   # adjust to your actual cities
    - "Santa Fe, NM"

salary:
  floor_hourly: 20.0      # your minimum acceptable hourly rate

profile:
  linkedin_url: "https://linkedin.com/in/your-profile"
  github_url: "https://github.com/your-username"
```

Everything else can stay as defaults for the first run.

---

## 5. Create your `portals.yaml`

```bash
cp portals.example.yaml portals.yaml
```

Open it — the example has ~20 companies pre-seeded (Cloudflare, Stripe, Linear, etc.).
Add, remove, or leave as-is. You can always edit later.
Just make sure at least a few entries are there so the first run returns something.

To add a company:
1. Find their careers page
2. If the URL contains `greenhouse.io`, `ashby.com`, or `lever.co` — you're set
3. The slug is in the URL: `boards.greenhouse.io/cloudflare` → slug is `cloudflare`
4. Add an entry:
   ```yaml
   - name: "Company Name"
     ats: "greenhouse"   # or: ashby / lever
     slug: "their-slug"
   ```

---

## 6. Drop in your resumes

```
materials/resumes/resume_dev.pdf    ← your dev/software resume
materials/resumes/resume_it.pdf     ← your IT resume (if you have one)
```

Filenames must contain `dev` or `it` — the parser uses that to classify them.
If you only have one resume, copy it to both names, or use only the dev path and
update `config.yaml` to point both `resumes.dev` and `resumes.it` at the same file.

---

## 7. Optional — cover letter samples

Drop 2–3 past cover letters as `.txt` files into `materials/cover_letter_samples/`.
The AI uses them to match your tone and voice. Safe to skip for the first run.

---

## 8. Install dependencies

```bash
# Python
pip3 install --break-system-packages -r requirements.txt

# Go
cd core && go mod tidy && cd ..
```

---

## 9. Run it

```bash
cd core && go run cmd/main.go
```

You should see:

```
main: web UI at http://localhost:8080
main: discord: connected        ← or "notifications disabled" if no credentials
main: watcher: watching ../materials/resumes
```

---

## 10. Trigger your first run

Open `http://localhost:8080`. Click **Run Now** in the top right.

The terminal shows scrape progress — which companies returned jobs, how many
passed scoring, etc. After 30–60 seconds refresh the page and jobs appear.

From there:
- Read the job cards
- **Reject** what doesn't fit — gone forever
- **Approve** what does → cover letter generated automatically
- Review and edit the letter
- Click **Submit Application** → the job URL opens for manual application

---

## How the automated schedule works

The app also runs on the cron schedule in `config.yaml` (default: daily at 7am).
When it fires, it scrapes all sources, scores results, and pings you on Discord
with a count and a link to the digest. **Run Now** does the exact same thing
on demand without waiting.

---

## Keeping it running persistently

To run without a terminal open, use `tmux`:

```bash
tmux new -s job-dispatch
cd /path/to/job-app-dispatch/core && go run cmd/main.go
# Ctrl+B then D to detach — keeps running in background
# tmux attach -t job-dispatch  to return to it
```

---

## Troubleshooting

| Symptom | Fix |
|---|---|
| `portals.yaml not found` | `cp portals.example.yaml portals.yaml` |
| `no module named requests` | `pip3 install --break-system-packages -r requirements.txt` |
| Discord not connecting | Check `DISCORD_TOKEN` in `.env`; verify Message Content Intent is on |
| No jobs appearing | Check terminal for scraper errors; verify a portals.yaml slug by opening `https://boards-api.greenhouse.io/v1/boards/{slug}/jobs` in a browser |
| Cover letter generation fails | Check `ANTHROPIC_API_KEY` is set correctly in `.env` |
