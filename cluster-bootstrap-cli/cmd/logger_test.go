package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLogger_NewLogger creates a new logger instance.
func TestLogger_NewLogger(t *testing.T) {
	logger := NewLogger(true)
	require.NotNil(t, logger)
	assert.True(t, logger.verbose)
	assert.Empty(t, logger.stages)
}

// TestLogger_Stage tests stage creation and logging.
func TestLogger_Stage(t *testing.T) {
	logger := NewLogger(false)
	stage := logger.Stage("Test Stage")
	require.NotNil(t, stage)
	assert.Equal(t, "Test Stage", stage.name)
	assert.NotZero(t, stage.start)
}

// TestStageLogger_Detail tests adding details to a stage.
func TestStageLogger_Detail(t *testing.T) {
	logger := NewLogger(false)
	stage := logger.Stage("Test Stage")

	stage.Detail("Detail 1")
	stage.Detail("Detail 2: %v", 42)

	assert.Len(t, stage.details, 2)
	assert.Equal(t, "Detail 1", stage.details[0])
	assert.Equal(t, "Detail 2: 42", stage.details[1])
}

// TestStageLogger_SecretDetail tests logging secret details without exposing values.
func TestStageLogger_SecretDetail(t *testing.T) {
	logger := NewLogger(false)
	stage := logger.Stage("Test Stage")

	stage.SecretDetail("Created", "my-secret", "argocd")

	assert.Len(t, stage.details, 1)
	assert.Contains(t, stage.details[0], "Created secret 'my-secret' in namespace 'argocd'")
}

// TestStageLogger_Done tests stage completion and duration recording.
func TestStageLogger_Done(t *testing.T) {
	logger := NewLogger(false)
	stage := logger.Stage("Test Stage")

	// Add a small sleep to ensure measurable duration
	time.Sleep(10 * time.Millisecond)
	stage.Done()

	require.Len(t, logger.stages, 1)
	stageLog := logger.stages[0]
	assert.Equal(t, "Test Stage", stageLog.Name)
	assert.Greater(t, stageLog.Duration, time.Duration(0))
	assert.NotZero(t, stageLog.StartTime)
}

// TestLogger_MultipleStages tests logging multiple stages with timings.
func TestLogger_MultipleStages(t *testing.T) {
	logger := NewLogger(false)

	// Stage 1
	stage1 := logger.Stage("Stage 1")
	stage1.Detail("Step 1")
	time.Sleep(10 * time.Millisecond)
	stage1.Done()

	// Stage 2
	stage2 := logger.Stage("Stage 2")
	stage2.Detail("Step 2")
	time.Sleep(10 * time.Millisecond)
	stage2.Done()

	require.Len(t, logger.stages, 2)
	assert.Equal(t, "Stage 1", logger.stages[0].Name)
	assert.Equal(t, "Stage 2", logger.stages[1].Name)
	assert.Greater(t, logger.stages[0].Duration, time.Duration(0))
	assert.Greater(t, logger.stages[1].Duration, time.Duration(0))
}

// TestLogger_GetStageSummary tests generating a summary with timings.
func TestLogger_GetStageSummary(t *testing.T) {
	logger := NewLogger(false)

	stage1 := logger.Stage("Stage 1")
	time.Sleep(10 * time.Millisecond)
	stage1.Done()

	stage2 := logger.Stage("Stage 2")
	time.Sleep(10 * time.Millisecond)
	stage2.Done()

	summary := logger.GetStageSummary()
	assert.NotEmpty(t, summary)
	assert.Contains(t, summary, "Bootstrap Timeline")
	assert.Contains(t, summary, "Stage 1")
	assert.Contains(t, summary, "Stage 2")
	assert.Contains(t, summary, "TOTAL")
}

// TestLogger_VerboseMode tests verbose logging output.
func TestLogger_VerboseMode(t *testing.T) {
	logger := NewLogger(true)
	stage := logger.Stage("Verbose Test")
	stage.Detail("Test detail")
	stage.Done()

	// In verbose mode, the logger should output details (captured implicitly)
	assert.Len(t, logger.stages, 1)
}

// TestLogger_EmptyStageSummary tests summary for empty logger.
func TestLogger_EmptyStageSummary(t *testing.T) {
	logger := NewLogger(false)
	summary := logger.GetStageSummary()
	assert.Empty(t, summary)
}

// TestLogger_DetailFormatting tests detail formatting with various formats.
func TestLogger_DetailFormatting(t *testing.T) {
	logger := NewLogger(false)
	stage := logger.Stage("Format Test")

	stage.Detail("String: %s", "value")
	stage.Detail("Integer: %d", 123)
	stage.Detail("Float: %.2f", 3.14159)
	stage.Detail("Boolean: %v", true)

	assert.Equal(t, "String: value", stage.details[0])
	assert.Equal(t, "Integer: 123", stage.details[1])
	assert.Equal(t, "Float: 3.14", stage.details[2])
	assert.Equal(t, "Boolean: true", stage.details[3])
}

// TestLogger_DetailCounting tests that details are properly counted.
func TestLogger_DetailCounting(t *testing.T) {
	logger := NewLogger(false)
	stage := logger.Stage("Count Test")

	for i := 0; i < 5; i++ {
		stage.Detail("Detail %d", i)
	}

	assert.Len(t, stage.details, 5)
}

// TestLogger_SummaryFormatting tests summary formatting.
func TestLogger_SummaryFormatting(t *testing.T) {
	logger := NewLogger(false)

	stage := logger.Stage("Duration Test")
	stage.Detail("Test")
	time.Sleep(50 * time.Millisecond)
	stage.Done()

	summary := logger.GetStageSummary()

	// Summary should include timing information
	assert.Contains(t, summary, "ms")
	assert.Contains(t, summary, "Duration Test")

	// Check that durations are formatted correctly
	lines := strings.Split(summary, "\n")
	found := false
	for _, line := range lines {
		if strings.Contains(line, "Duration Test") {
			assert.Contains(t, line, "ms")
			found = true
			break
		}
	}
	assert.True(t, found, "Duration Test not found in summary")
}
