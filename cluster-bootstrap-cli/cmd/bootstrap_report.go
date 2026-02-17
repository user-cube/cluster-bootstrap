package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// BootstrapReport captures the complete state and metrics of a bootstrap operation.
type BootstrapReport struct {
	Environment   string         `json:"environment"`
	StartTime     time.Time      `json:"start_time"`
	EndTime       time.Time      `json:"end_time"`
	Duration      string         `json:"duration"`
	DurationMs    int64          `json:"duration_ms"`
	Success       bool           `json:"success"`
	Stages        []StageReport  `json:"stages"`
	Resources     ResourceReport `json:"resources"`
	Health        *HealthReport  `json:"health,omitempty"`
	Configuration ConfigReport   `json:"configuration"`
	Error         string         `json:"error,omitempty"`
}

// StageReport captures metrics for a single bootstrap stage.
type StageReport struct {
	Name       string    `json:"name"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	Duration   string    `json:"duration"`
	DurationMs int64     `json:"duration_ms"`
	Success    bool      `json:"success"`
	Details    []string  `json:"details,omitempty"`
	Error      string    `json:"error,omitempty"`
}

// ResourceReport captures information about created/updated resources.
type ResourceReport struct {
	Namespace     NamespaceReport   `json:"namespace"`
	Secrets       []SecretReport    `json:"secrets"`
	ArgoCDRelease HelmReleaseReport `json:"argocd_release"`
	AppOfApps     ApplicationReport `json:"app_of_apps"`
}

// NamespaceReport captures namespace creation info.
type NamespaceReport struct {
	Name    string `json:"name"`
	Created bool   `json:"created"`
}

// SecretReport captures secret creation/update info.
type SecretReport struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Created   bool   `json:"created"` // true = created, false = updated
}

// HelmReleaseReport captures Helm release info.
type HelmReleaseReport struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Installed bool   `json:"installed"` // true = installed, false = upgraded
	Skipped   bool   `json:"skipped"`
}

// ApplicationReport captures ArgoCD Application info.
type ApplicationReport struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Created   bool   `json:"created"` // true = created, false = updated
}

// HealthReport captures post-bootstrap health check results.
type HealthReport struct {
	Checked    bool              `json:"checked"`
	Healthy    bool              `json:"healthy"`
	Components []ComponentHealth `json:"components"`
	Timeout    int               `json:"timeout_seconds"`
}

// ComponentHealth captures individual component health status.
type ComponentHealth struct {
	Name   string `json:"name"`
	Status string `json:"status"` // Ready, NotReady, NotInstalled, Unknown
}

// ConfigReport captures configuration used for bootstrap.
type ConfigReport struct {
	BaseDir           string `json:"base_dir"`
	AppPath           string `json:"app_path"`
	Encryption        string `json:"encryption"`
	SecretsFile       string `json:"secrets_file"`
	Kubeconfig        string `json:"kubeconfig,omitempty"`
	Context           string `json:"context,omitempty"`
	DryRun            bool   `json:"dry_run"`
	SkipArgoCDInstall bool   `json:"skip_argocd_install"`
	WaitForHealth     bool   `json:"wait_for_health"`
}

// NewBootstrapReport creates a new bootstrap report.
func NewBootstrapReport(env string) *BootstrapReport {
	return &BootstrapReport{
		Environment: env,
		StartTime:   time.Now(),
		Stages:      []StageReport{},
		Resources: ResourceReport{
			Secrets: []SecretReport{},
		},
	}
}

// AddStage adds a completed stage to the report.
func (r *BootstrapReport) AddStage(stage StageReport) {
	r.Stages = append(r.Stages, stage)
}

// Complete finalizes the report with end time and duration.
func (r *BootstrapReport) Complete(success bool, err error) {
	r.EndTime = time.Now()
	r.Success = success
	duration := r.EndTime.Sub(r.StartTime)
	r.Duration = duration.Round(time.Millisecond).String()
	r.DurationMs = duration.Milliseconds()
	if err != nil {
		r.Error = err.Error()
	}
}

// ToJSON serializes the report to JSON.
func (r *BootstrapReport) ToJSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal report to JSON: %w", err)
	}
	return string(data), nil
}

// WriteToFile writes the report to a file in JSON format.
func (r *BootstrapReport) WriteToFile(path string) error {
	jsonData, err := r.ToJSON()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(jsonData), 0600); err != nil {
		return fmt.Errorf("failed to write report to %s: %w", path, err)
	}
	return nil
}

// PrintSummary prints a human-readable summary of the report.
func (r *BootstrapReport) PrintSummary() {
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("📊 Bootstrap Report")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Status
	status := "✅ SUCCESS"
	if !r.Success {
		status = "❌ FAILED"
	}
	fmt.Printf("Status:       %s\n", status)
	fmt.Printf("Environment:  %s\n", r.Environment)
	fmt.Printf("Duration:     %s\n", r.Duration)
	fmt.Printf("Encryption:   %s\n", r.Configuration.Encryption)

	// Stages
	fmt.Println()
	fmt.Println("⏱️  Stages:")
	for _, stage := range r.Stages {
		stageStatus := "✓"
		if !stage.Success {
			stageStatus = "✗"
		}
		fmt.Printf("  %s %-30s %8s\n", stageStatus, stage.Name, stage.Duration)
	}

	// Resources
	fmt.Println()
	fmt.Println("📦 Resources:")
	fmt.Printf("  Namespace:     %s (%s)\n", r.Resources.Namespace.Name, statusText(r.Resources.Namespace.Created, "created", "verified"))

	for _, secret := range r.Resources.Secrets {
		fmt.Printf("  Secret:        %s/%s (%s)\n", secret.Namespace, secret.Name, statusText(secret.Created, "created", "updated"))
	}

	if !r.Resources.ArgoCDRelease.Skipped {
		fmt.Printf("  Helm Release:  %s (%s)\n", r.Resources.ArgoCDRelease.Name, statusText(r.Resources.ArgoCDRelease.Installed, "installed", "upgraded"))
	} else {
		fmt.Printf("  Helm Release:  %s (skipped)\n", r.Resources.ArgoCDRelease.Name)
	}

	fmt.Printf("  Application:   %s (%s)\n", r.Resources.AppOfApps.Name, statusText(r.Resources.AppOfApps.Created, "created", "updated"))

	// Health checks
	if r.Health != nil && r.Health.Checked {
		fmt.Println()
		fmt.Println("💚 Health Checks:")
		healthStatus := "PASSED"
		if !r.Health.Healthy {
			healthStatus = "FAILED"
		}
		fmt.Printf("  Overall: %s\n", healthStatus)
		for _, comp := range r.Health.Components {
			icon := "✓"
			if comp.Status != "Ready" && comp.Status != "NotInstalled" {
				icon = "✗"
			}
			fmt.Printf("  %s %-20s %s\n", icon, comp.Name, comp.Status)
		}
	}

	if r.Error != "" {
		fmt.Println()
		fmt.Println("❌ Error:")
		fmt.Printf("  %s\n", r.Error)
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

// statusText returns the appropriate status text based on boolean.
func statusText(isNew bool, newText, existingText string) string {
	if isNew {
		return newText
	}
	return existingText
}

// newStageTimer creates a helper for timing stages.
type stageTimer struct {
	name      string
	startTime time.Time
	details   []string
}

func startStage(name string) *stageTimer {
	return &stageTimer{
		name:      name,
		startTime: time.Now(),
		details:   []string{},
	}
}

func (s *stageTimer) addDetail(detail string) {
	s.details = append(s.details, detail)
}

func (s *stageTimer) complete(success bool, err error) StageReport {
	endTime := time.Now()
	duration := endTime.Sub(s.startTime)

	report := StageReport{
		Name:       s.name,
		StartTime:  s.startTime,
		EndTime:    endTime,
		Duration:   duration.Round(time.Millisecond).String(),
		DurationMs: duration.Milliseconds(),
		Success:    success,
		Details:    s.details,
	}

	if err != nil {
		report.Error = err.Error()
	}

	return report
}
