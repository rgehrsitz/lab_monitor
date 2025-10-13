package scheduler

import (
	"context"
	"fmt"
	"time"

	gocron "github.com/go-co-op/gocron/v2"
)

type Scheduler struct {
	inner gocron.Scheduler
}

func New(location *time.Location) (*Scheduler, error) {
	s, err := gocron.NewScheduler(gocron.WithLocation(location))
	if err != nil {
		return nil, err
	}
	return &Scheduler{inner: s}, nil
}

func (s *Scheduler) ScheduleDaily(times []string, task func(context.Context) error) error {
	if len(times) == 0 {
		return fmt.Errorf("no schedule times provided")
	}
	ats := make([]gocron.AtTime, 0, len(times))
	for _, t := range times {
		parsed, err := time.Parse("15:04", t)
		if err != nil {
			return fmt.Errorf("parse schedule time %s: %w", t, err)
		}
		ats = append(ats, gocron.NewAtTime(uint(parsed.Hour()), uint(parsed.Minute()), 0))
	}
	first := ats[0]
	var jobTimes gocron.AtTimes
	if len(ats) == 1 {
		jobTimes = gocron.NewAtTimes(first)
	} else {
		jobTimes = gocron.NewAtTimes(first, ats[1:]...)
	}
	_, err := s.inner.NewJob(
		gocron.DailyJob(1, jobTimes),
		gocron.NewTask(
			func(ctx context.Context) {
				_ = task(ctx)
			},
		),
		gocron.WithSingletonMode(gocron.LimitModeWait),
	)
	return err
}

func (s *Scheduler) Start(ctx context.Context) error {
	s.inner.Start()
	<-ctx.Done()
	return s.inner.Shutdown()
}
