package discord

import (
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
	"github.com/thomas-tahk/job-app-dispatch/internal/models"
)

// Bot sends notifications to a single Discord channel.
// It does not implement slash commands or interactive buttons —
// all interactive actions happen in the local web UI.
//
// If token or channelID are empty, the Bot runs in disabled mode:
// all methods are no-ops so the rest of the app works without Discord.
type Bot struct {
	session   *discordgo.Session
	channelID string
	disabled  bool
}

func New(token, channelID string) (*Bot, error) {
	if token == "" || channelID == "" {
		log.Println("discord: no credentials configured — notifications disabled")
		return &Bot{disabled: true}, nil
	}
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("discord: create session: %w", err)
	}
	return &Bot{session: s, channelID: channelID}, nil
}

func (b *Bot) Open() error {
	if b.disabled {
		return nil
	}
	return b.session.Open()
}

func (b *Bot) Close() error {
	if b.disabled {
		return nil
	}
	return b.session.Close()
}

// NotifyDigestReady pings the channel when new jobs are ready to review in the web UI.
func (b *Bot) NotifyDigestReady(count int, webAddr string) error {
	if b.disabled {
		log.Printf("discord(disabled): %d new job(s) ready — %s", count, webAddr)
		return nil
	}
	msg := fmt.Sprintf("**%d new job(s) ready for review** → %s", count, webAddr)
	_, err := b.session.ChannelMessageSend(b.channelID, msg)
	return err
}

// NotifySubmissionSuccess confirms a successful automated submission.
func (b *Bot) NotifySubmissionSuccess(job models.Job) error {
	if b.disabled {
		log.Printf("discord(disabled): applied to %s at %s", job.Title, job.Company)
		return nil
	}
	msg := fmt.Sprintf("Applied to **%s** at %s", job.Title, job.Company)
	_, err := b.session.ChannelMessageSend(b.channelID, msg)
	return err
}

// NotifySubmissionFailed alerts when automation couldn't handle a form.
// The user must apply manually via the provided link.
func (b *Bot) NotifySubmissionFailed(job models.Job, manualURL string) error {
	if b.disabled {
		log.Printf("discord(disabled): submission failed for %s — %s", job.Title, manualURL)
		return nil
	}
	msg := fmt.Sprintf(
		"Submission failed for **%s** at %s — apply manually: %s",
		job.Title, job.Company, manualURL,
	)
	_, err := b.session.ChannelMessageSend(b.channelID, msg)
	return err
}
