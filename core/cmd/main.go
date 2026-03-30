package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/thomas-tahk/job-app-dispatch/internal/ai"
	"github.com/thomas-tahk/job-app-dispatch/internal/db"
	"github.com/thomas-tahk/job-app-dispatch/internal/discord"
	"github.com/thomas-tahk/job-app-dispatch/internal/models"
	"github.com/thomas-tahk/job-app-dispatch/internal/scheduler"
	"github.com/thomas-tahk/job-app-dispatch/internal/watcher"
	"github.com/thomas-tahk/job-app-dispatch/internal/web"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

type Config struct {
	Schedule struct {
		Cron string `yaml:"cron"`
	} `yaml:"schedule"`
	Web struct {
		Addr string `yaml:"addr"`
	} `yaml:"web"`
	Profile struct {
		LinkedInURL string `yaml:"linkedin_url"`
		GitHubURL   string `yaml:"github_url"`
	} `yaml:"profile"`
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("main: no .env file, reading from environment")
	}

	cfgBytes, err := os.ReadFile("../config.yaml")
	if err != nil {
		log.Fatalf("main: read config: %v", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(cfgBytes, &cfg); err != nil {
		log.Fatalf("main: parse config: %v", err)
	}
	if cfg.Web.Addr == "" {
		cfg.Web.Addr = ":8080"
	}

	database, err := db.Open("../data/jobs.db")
	if err != nil {
		log.Fatalf("main: db: %v", err)
	}

	discordBot, err := discord.New(
		os.Getenv("DISCORD_TOKEN"),
		os.Getenv("DISCORD_CHANNEL_ID"),
	)
	if err != nil {
		log.Fatalf("main: discord: %v", err)
	}
	if err := discordBot.Open(); err != nil {
		log.Fatalf("main: discord open: %v", err)
	}
	defer discordBot.Close()

	aiClient := ai.New(os.Getenv("ANTHROPIC_API_KEY"))

	sched := scheduler.New()
	if err := sched.AddJob("scrape", cfg.Schedule.Cron, func() {
		runScrapeAndScore(database, aiClient, discordBot, cfg)
	}); err != nil {
		log.Fatalf("main: scheduler: %v", err)
	}
	sched.Start()
	defer sched.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resumeWatcher, err := watcher.NewResumeWatcher("../materials/resumes", func(path string) {
		parseAndStoreResume(database, path)
	})
	if err != nil {
		log.Fatalf("main: watcher: %v", err)
	}
	if err := resumeWatcher.Start(ctx); err != nil {
		log.Fatalf("main: watcher start: %v", err)
	}

	server, err := web.New(database, cfg.Web.Addr)
	if err != nil {
		log.Fatalf("main: web server init: %v", err)
	}
	go func() {
		log.Printf("main: web UI at http://localhost%s", cfg.Web.Addr)
		if err := server.Start(); err != nil {
			log.Fatalf("main: web server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("main: shutting down")
}

// parseAndStoreResume calls the Python resume parser and upserts the result in the DB.
func parseAndStoreResume(database *gorm.DB, path string) {
	out, err := exec.Command("python", "../scripts/parse_resume.py", path).Output()
	if err != nil {
		log.Printf("parseAndStoreResume: parse failed for %s: %v", path, err)
		return
	}
	var result struct {
		Filename   string   `json:"filename"`
		RoleType   string   `json:"role_type"`
		ParsedText string   `json:"parsed_text"`
		Skills     []string `json:"skills"`
		Titles     []string `json:"titles"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		log.Printf("parseAndStoreResume: bad JSON for %s: %v", path, err)
		return
	}
	skillsJSON, _ := json.Marshal(result.Skills)
	titlesJSON, _ := json.Marshal(result.Titles)
	resume := models.Resume{
		Filename:   result.Filename,
		RoleType:   models.RoleType(result.RoleType),
		ParsedText: result.ParsedText,
		Skills:     string(skillsJSON),
		Titles:     string(titlesJSON),
	}
	database.Where(models.Resume{Filename: result.Filename}).Assign(resume).FirstOrCreate(&resume)
	log.Printf("parseAndStoreResume: stored resume %s", result.Filename)
}

// runScrapeAndScore is the main pipeline: scrape → filter → score → store → notify.
// TODO: implement fully; currently a stub.
func runScrapeAndScore(database *gorm.DB, aiClient *ai.Client, bot *discord.Bot, cfg Config) {
	log.Println("pipeline: starting scrape run")
	// TODO:
	// 1. Run each connector's scraper subprocess
	// 2. Deduplicate against DB (by ExternalID + Source)
	// 3. Filter: salary, healthcare, location rules
	// 4. Score each job
	// 5. Generate match rationale via aiClient
	// 6. Store new jobs in DB
	// 7. Count new jobs, send Discord notification if > 0
	log.Println("pipeline: scrape run complete (not yet implemented)")
}
