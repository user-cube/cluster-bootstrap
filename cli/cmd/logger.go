package cmd

import (
	"fmt"
	"time"
)

// Logger provides structured logging with optional timestamp support.
type Logger struct {
	verbose bool
	stages  []StageLog
}

// StageLog records information about a stage execution.
type StageLog struct {
	Name      string
	StartTime time.Time
	Duration  time.Duration
	Details   []string
}

// NewLogger creates a new logger instance.
func NewLogger(verbose bool) *Logger {
	return &Logger{
		verbose: verbose,
		stages:  make([]StageLog, 0),
	}
}

// Stage starts logging a new stage.
func (l *Logger) Stage(name string) *StageLogger {
	return &StageLogger{
		logger: l,
		name:   name,
		start:  time.Now(),
	}
}

// StageLogger is a helper for logging a single stage.
type StageLogger struct {
	logger  *Logger
	name    string
	start   time.Time
	details []string
}

// Detail adds a detail line to the current stage.
func (s *StageLogger) Detail(format string, args ...interface{}) {
	detail := fmt.Sprintf(format, args...)
	s.details = append(s.details, detail)
	if s.logger.verbose {
		fmt.Printf("    • %s\n", detail)
	}
}

// SecretDetail logs a secret-related operation without exposing values.
func (s *StageLogger) SecretDetail(operation, secretName, namespace string) {
	detail := fmt.Sprintf("%s secret '%s' in namespace '%s'", operation, secretName, namespace)
	s.details = append(s.details, detail)
	if s.logger.verbose {
		fmt.Printf("    • 🔐 %s\n", detail)
	}
}

// Done marks the stage as complete and records its duration.
func (s *StageLogger) Done() {
	duration := time.Since(s.start)
	stage := StageLog{
		Name:      s.name,
		StartTime: s.start,
		Duration:  duration,
		Details:   s.details,
	}
	s.logger.stages = append(s.logger.stages, stage)

	if s.logger.verbose && duration > 100*time.Millisecond {
		fmt.Printf("    ⏱ completed in %v\n", duration.Round(time.Millisecond))
	}
}

// GetStageSummary returns a summary of all stages with timings.
func (l *Logger) GetStageSummary() string {
	if len(l.stages) == 0 {
		return ""
	}

	var summary string
	totalDuration := time.Duration(0)

	summary += "\n=== Bootstrap Timeline ===\n"
	for _, stage := range l.stages {
		duration := stage.Duration.Round(time.Millisecond)
		totalDuration += duration
		summary += fmt.Sprintf("  %s: %v\n", stage.Name, duration)
	}
	summary += fmt.Sprintf("  TOTAL: %v\n", totalDuration.Round(time.Millisecond))

	return summary
}

// PrintStageSummary prints the stage summary if verbose is enabled.
func (l *Logger) PrintStageSummary() {
	if l.verbose {
		fmt.Print(l.GetStageSummary())
	}
}
