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
	"github.com/thomas-tahk/job-app-dispatch/internal/connector"
	"github.com/thomas-tahk/job-app-dispatch/internal/db"
	"github.com/thomas-tahk/job-app-dispatch/internal/discord"
	"github.com/thomas-tahk/job-app-dispatch/internal/models"
	"github.com/thomas-tahk/job-app-dispatch/internal/pipeline"
	"github.com/thomas-tahk/job-app-dispatch/internal/scheduler"
	"github.com/thomas-tahk/job-app-dispatch/internal/scorer"
	"github.com/thomas-tahk/job-app-dispatch/internal/watcher"
	"github.com/thomas-tahk/job-app-dispatch/internal/web"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

type Config struct {
	Schedule struct {
		Cron string `yaml:"cron"`
	} `yaml:"schedule"`
	Locations struct {
		OnsiteAllowed []string `yaml:"onsite_allowed"`
	} `yaml:"locations"`
	Salary struct {
		FloorHourly float64 `yaml:"floor_hourly"`
	} `yaml:"salary"`
	Targets struct {
		RoleKeywords struct {
			Dev []string `yaml:"dev"`
			IT  []string `yaml:"it"`
		} `yaml:"role_keywords"`
	} `yaml:"targets"`
	Web struct {
		Addr string `yaml:"addr"`
	} `yaml:"web"`
	Resumes struct {
		Dev string `yaml:"dev"`
		IT  string `yaml:"it"`
	} `yaml:"resumes"`
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

	runner := &pipeline.Runner{
		DB: database,
		Scrapers: []connector.Scraper{
			// ATS API scraper: Greenhouse, Ashby, Lever (reads portals.yaml).
			connector.NewPythonScraper("ats", "../connectors/ats/scraper.py", "../config.yaml"),
			// Broad discovery: free job board APIs, no auth required.
			connector.NewPythonScraper("remoteok", "../connectors/remoteok/scraper.py", "../config.yaml"),
			connector.NewPythonScraper("arbeitnow", "../connectors/arbeitnow/scraper.py", "../config.yaml"),
		},
		Submitters: map[string]connector.Submitter{},
		AI:      ai.New(os.Getenv("ANTHROPIC_API_KEY")),
		Discord: discordBot,
		Config: pipeline.Config{
			WebAddr:       cfg.Web.Addr,
			LinkedInURL:   cfg.Profile.LinkedInURL,
			GitHubURL:     cfg.Profile.GitHubURL,
			DevResumePath: "../" + cfg.Resumes.Dev,
			ITResumePath:  "../" + cfg.Resumes.IT,
			SamplesDir:    "../materials/cover_letter_samples",
			Scorer: scorer.Config{
				SalaryFloorHourly:      cfg.Salary.FloorHourly,
				OnsiteAllowedLocations: cfg.Locations.OnsiteAllowed,
				DevKeywords:            cfg.Targets.RoleKeywords.Dev,
				ITKeywords:             cfg.Targets.RoleKeywords.IT,
			},
		},
	}

	sched := scheduler.New()
	if err := sched.AddJob("scrape", cfg.Schedule.Cron, func() {
		runner.Run(context.Background())
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

	server, err := web.New(
		database,
		cfg.Web.Addr,
		runner.ProcessApproval,
		runner.Submit,
		func() { runner.Run(context.Background()) },
		runner.IngestURL,
	)
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
	out, err := exec.Command("python3", "../scripts/parse_resume.py", path).Output()
	if err != nil {
		log.Printf("parseAndStoreResume: failed for %s: %v", path, err)
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
	log.Printf("parseAndStoreResume: stored %s", result.Filename)
}
