package scheduler

import (
	"log"

	"github.com/robfig/cron/v3"
)

// Scheduler wraps robfig/cron with a simpler interface.
type Scheduler struct {
	c *cron.Cron
}

func New() *Scheduler {
	return &Scheduler{c: cron.New()}
}

// AddJob registers a named function on a cron schedule expression.
// Example schedule: "0 7 * * *" (daily at 7 AM).
func (s *Scheduler) AddJob(name, schedule string, fn func()) error {
	_, err := s.c.AddFunc(schedule, fn)
	if err != nil {
		return err
	}
	log.Printf("scheduler: registered %q on schedule %q", name, schedule)
	return nil
}

func (s *Scheduler) Start() {
	s.c.Start()
	log.Println("scheduler: started")
}

func (s *Scheduler) Stop() {
	s.c.Stop()
}
