package worker

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	// ErrInvalidCronExpression is returned for malformed cron expressions.
	ErrInvalidCronExpression = errors.New("invalid cron expression")
	// ErrSchedulerStopped is returned when the scheduler is not running.
	ErrSchedulerStopped = errors.New("scheduler is stopped")
)

// CronField represents a single field in a cron expression.
type CronField struct {
	min, max int
	values   map[int]bool
}

// NewCronField creates a new cron field with the given range.
func NewCronField(min, max int) *CronField {
	return &CronField{
		min:    min,
		max:    max,
		values: make(map[int]bool),
	}
}

// Parse parses a cron field expression.
// Supports: *, */n, n, n,m,o, n-m
func (cf *CronField) Parse(expr string) error {
	cf.values = make(map[int]bool)

	// Handle wildcard
	if expr == "*" {
		for i := cf.min; i <= cf.max; i++ {
			cf.values[i] = true
		}
		return nil
	}

	// Handle step values (*/n or n/m)
	if strings.Contains(expr, "/") {
		parts := strings.Split(expr, "/")
		if len(parts) != 2 {
			return ErrInvalidCronExpression
		}

		step, err := strconv.Atoi(parts[1])
		if err != nil || step <= 0 {
			return ErrInvalidCronExpression
		}

		start := cf.min
		end := cf.max

		if parts[0] != "*" {
			// Handle range/step like 1-10/2
			if strings.Contains(parts[0], "-") {
				rangeParts := strings.Split(parts[0], "-")
				if len(rangeParts) != 2 {
					return ErrInvalidCronExpression
				}
				start, err = strconv.Atoi(rangeParts[0])
				if err != nil {
					return ErrInvalidCronExpression
				}
				end, err = strconv.Atoi(rangeParts[1])
				if err != nil {
					return ErrInvalidCronExpression
				}
			} else {
				start, err = strconv.Atoi(parts[0])
				if err != nil {
					return ErrInvalidCronExpression
				}
			}
		}

		for i := start; i <= end; i += step {
			if i >= cf.min && i <= cf.max {
				cf.values[i] = true
			}
		}
		return nil
	}

	// Handle comma-separated values
	if strings.Contains(expr, ",") {
		parts := strings.Split(expr, ",")
		for _, part := range parts {
			// Each part could be a range or single value
			if strings.Contains(part, "-") {
				if err := cf.parseRange(part); err != nil {
					return err
				}
			} else {
				val, err := strconv.Atoi(strings.TrimSpace(part))
				if err != nil {
					return ErrInvalidCronExpression
				}
				if val >= cf.min && val <= cf.max {
					cf.values[val] = true
				}
			}
		}
		return nil
	}

	// Handle range (n-m)
	if strings.Contains(expr, "-") {
		return cf.parseRange(expr)
	}

	// Handle single value
	val, err := strconv.Atoi(expr)
	if err != nil {
		return ErrInvalidCronExpression
	}
	if val < cf.min || val > cf.max {
		return ErrInvalidCronExpression
	}
	cf.values[val] = true
	return nil
}

// parseRange parses a range expression (n-m).
func (cf *CronField) parseRange(expr string) error {
	parts := strings.Split(expr, "-")
	if len(parts) != 2 {
		return ErrInvalidCronExpression
	}

	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return ErrInvalidCronExpression
	}

	end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return ErrInvalidCronExpression
	}

	if start > end || start < cf.min || end > cf.max {
		return ErrInvalidCronExpression
	}

	for i := start; i <= end; i++ {
		cf.values[i] = true
	}
	return nil
}

// Contains returns true if the value is in this field.
func (cf *CronField) Contains(val int) bool {
	return cf.values[val]
}

// Next returns the next valid value >= val, or -1 if none.
func (cf *CronField) Next(val int) int {
	for i := val; i <= cf.max; i++ {
		if cf.values[i] {
			return i
		}
	}
	return -1
}

// First returns the first valid value in this field.
func (cf *CronField) First() int {
	for i := cf.min; i <= cf.max; i++ {
		if cf.values[i] {
			return i
		}
	}
	return cf.min
}

// CronSchedule represents a parsed cron expression.
type CronSchedule struct {
	Minute  *CronField
	Hour    *CronField
	Day     *CronField
	Month   *CronField
	Weekday *CronField
	Raw     string
}

// ParseCron parses a cron expression string.
// Format: minute hour day month weekday
func ParseCron(expr string) (*CronSchedule, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, ErrInvalidCronExpression
	}

	schedule := &CronSchedule{
		Minute:  NewCronField(0, 59),
		Hour:    NewCronField(0, 23),
		Day:     NewCronField(1, 31),
		Month:   NewCronField(1, 12),
		Weekday: NewCronField(0, 6), // 0 = Sunday
		Raw:     expr,
	}

	if err := schedule.Minute.Parse(fields[0]); err != nil {
		return nil, err
	}
	if err := schedule.Hour.Parse(fields[1]); err != nil {
		return nil, err
	}
	if err := schedule.Day.Parse(fields[2]); err != nil {
		return nil, err
	}
	if err := schedule.Month.Parse(fields[3]); err != nil {
		return nil, err
	}
	if err := schedule.Weekday.Parse(fields[4]); err != nil {
		return nil, err
	}

	return schedule, nil
}

// Next returns the next time this schedule should run after the given time.
func (cs *CronSchedule) Next(from time.Time) time.Time {
	// Start from the next minute
	t := from.Truncate(time.Minute).Add(time.Minute)

	// Maximum iterations to prevent infinite loop
	maxIter := 366 * 24 * 60 // One year of minutes
	for i := 0; i < maxIter; i++ {
		// Check month
		if !cs.Month.Contains(int(t.Month())) {
			nextMonth := cs.Month.Next(int(t.Month()))
			if nextMonth == -1 {
				// Move to next year
				t = time.Date(t.Year()+1, time.Month(cs.Month.First()), 1, 0, 0, 0, 0, t.Location())
			} else {
				t = time.Date(t.Year(), time.Month(nextMonth), 1, 0, 0, 0, 0, t.Location())
			}
			continue
		}

		// Check day and weekday
		if !cs.Day.Contains(t.Day()) || !cs.Weekday.Contains(int(t.Weekday())) {
			t = t.AddDate(0, 0, 1)
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
			continue
		}

		// Check hour
		if !cs.Hour.Contains(t.Hour()) {
			nextHour := cs.Hour.Next(t.Hour())
			if nextHour == -1 {
				// Move to next day
				t = t.AddDate(0, 0, 1)
				t = time.Date(t.Year(), t.Month(), t.Day(), cs.Hour.First(), 0, 0, 0, t.Location())
			} else {
				t = time.Date(t.Year(), t.Month(), t.Day(), nextHour, 0, 0, 0, t.Location())
			}
			continue
		}

		// Check minute
		if !cs.Minute.Contains(t.Minute()) {
			nextMinute := cs.Minute.Next(t.Minute())
			if nextMinute == -1 {
				// Move to next hour
				t = t.Add(time.Hour)
				t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), cs.Minute.First(), 0, 0, t.Location())
			} else {
				t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), nextMinute, 0, 0, t.Location())
			}
			continue
		}

		// Found a match
		return t
	}

	// Fallback - should never reach here with valid cron
	return from.Add(time.Minute)
}

// ScheduledEntry represents a scheduled job entry.
type ScheduledEntry struct {
	ID       string
	Job      *Job
	Schedule *CronSchedule
	NextRun  time.Time
	LastRun  time.Time
}

// Scheduler manages scheduled jobs.
type Scheduler struct {
	entries   []*ScheduledEntry
	running   bool
	stopCh    chan struct{}
	submitFn  func(*Job) *Future
	mu        sync.RWMutex
	wg        sync.WaitGroup
	location  *time.Location
	idCounter int
}

// NewScheduler creates a new scheduler.
func NewScheduler(submitFn func(*Job) *Future) *Scheduler {
	return &Scheduler{
		entries:  make([]*ScheduledEntry, 0),
		submitFn: submitFn,
		location: time.Local,
	}
}

// SetLocation sets the timezone for the scheduler.
func (s *Scheduler) SetLocation(loc *time.Location) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.location = loc
}

// AddJob adds a job to run on the given schedule.
func (s *Scheduler) AddJob(cronExpr string, job *Job) (string, error) {
	schedule, err := ParseCron(cronExpr)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.idCounter++
	entryID := job.ID
	if entryID == "" {
		entryID = strconv.Itoa(s.idCounter)
	}

	entry := &ScheduledEntry{
		ID:       entryID,
		Job:      job,
		Schedule: schedule,
		NextRun:  schedule.Next(time.Now().In(s.location)),
	}

	s.entries = append(s.entries, entry)
	s.sortEntries()

	return entryID, nil
}

// AddFunc is a convenience method to add a function as a scheduled job.
func (s *Scheduler) AddFunc(cronExpr string, fn JobFunc) (string, error) {
	s.mu.Lock()
	s.idCounter++
	id := strconv.Itoa(s.idCounter)
	s.mu.Unlock()

	job := NewJob(id, fn)
	return s.AddJob(cronExpr, job)
}

// RemoveJob removes a scheduled job by ID.
func (s *Scheduler) RemoveJob(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, entry := range s.entries {
		if entry.ID == id {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			return true
		}
	}
	return false
}

// sortEntries sorts entries by next run time.
func (s *Scheduler) sortEntries() {
	sort.Slice(s.entries, func(i, j int) bool {
		return s.entries[i].NextRun.Before(s.entries[j].NextRun)
	})
}

// Start starts the scheduler.
func (s *Scheduler) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	s.wg.Add(1)
	go s.run()

	return nil
}

// run is the main scheduler loop.
func (s *Scheduler) run() {
	defer s.wg.Done()

	for {
		s.mu.RLock()
		if len(s.entries) == 0 {
			s.mu.RUnlock()
			select {
			case <-time.After(time.Second):
				continue
			case <-s.stopCh:
				return
			}
		}

		nextRun := s.entries[0].NextRun
		s.mu.RUnlock()

		now := time.Now().In(s.location)
		if nextRun.After(now) {
			select {
			case <-time.After(nextRun.Sub(now)):
				// Time to run
			case <-s.stopCh:
				return
			}
		}

		s.runPendingJobs()
	}
}

// runPendingJobs runs all jobs that are due.
func (s *Scheduler) runPendingJobs() {
	now := time.Now().In(s.location)

	s.mu.Lock()
	var toRun []*ScheduledEntry

	for _, entry := range s.entries {
		if !entry.NextRun.After(now) {
			toRun = append(toRun, entry)
			entry.LastRun = now
			entry.NextRun = entry.Schedule.Next(now)
		}
	}

	s.sortEntries()
	s.mu.Unlock()

	// Submit jobs for execution
	for _, entry := range toRun {
		if s.submitFn != nil {
			// Create a copy of the job for this execution
			job := NewJob(entry.Job.ID, entry.Job.Func).
				WithName(entry.Job.Name).
				WithPriority(entry.Job.Priority).
				WithTimeout(entry.Job.Timeout).
				WithRetry(entry.Job.MaxRetries, entry.Job.RetryDelay)
			s.submitFn(job)
		}
	}
}

// Stop stops the scheduler.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopCh)
	s.mu.Unlock()

	s.wg.Wait()
}

// Entries returns a copy of all scheduled entries.
func (s *Scheduler) Entries() []*ScheduledEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := make([]*ScheduledEntry, len(s.entries))
	copy(entries, s.entries)
	return entries
}

// IsRunning returns true if the scheduler is running.
func (s *Scheduler) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}
