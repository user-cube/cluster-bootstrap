package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/term"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// bootstrapCountdownSeconds is the grace period given to abort a bootstrap once
// the target cluster context has been announced.
const bootstrapCountdownSeconds = 10

// countdownInterval is the delay between countdown ticks. Overridden in tests.
var countdownInterval = time.Second

// appOfAppsGetter reads the App of Apps root Application from a cluster.
type appOfAppsGetter interface {
	GetAppOfApps(ctx context.Context) (*unstructured.Unstructured, error)
}

// announceTargetContext tells the operator which cluster is about to be modified.
// When interactive, it counts down so the bootstrap can still be aborted with Ctrl+C.
func announceTargetContext(out io.Writer, kubeContext string, seconds int, interactive bool) {
	fmt.Fprintf(out, "\n%s Bootstrap will modify the cluster on Kubernetes context: %s\n",
		warningColor("⚠ "), stepColor(kubeContext))

	if !interactive || seconds <= 0 {
		return
	}

	for remaining := seconds; remaining > 0; remaining-- {
		fmt.Fprintf(out, "\r    Starting in %2ds... press Ctrl+C to abort", remaining)
		time.Sleep(countdownInterval)
	}
	fmt.Fprintf(out, "\r    Starting now...                            \n")
}

// guardExistingAppOfApps refuses to bootstrap a cluster that already has an App
// of Apps root Application, unless force is set. Returns the existing
// Application when one was found so callers can report it.
func guardExistingAppOfApps(ctx context.Context, client appOfAppsGetter, kubeContext string, force bool) error {
	existing, err := client.GetAppOfApps(ctx)
	if err != nil {
		return err
	}
	if existing == nil {
		return nil
	}

	app := parseArgoCDApplication(existing)
	if force {
		warnf("An App of Apps already exists on context %s and will be overwritten (--force).", kubeContext)
		printExistingAppOfApps(os.Stdout, app)
		return nil
	}

	printExistingAppOfApps(os.Stdout, app)
	return fmt.Errorf("cluster already bootstrapped: App of Apps %q exists in namespace %s on context %s\n"+
		"  hint: inspect the existing installation with: cluster-bootstrap-cli info <environment>\n"+
		"  tip: re-run with --force to overwrite the existing App of Apps",
		app.Name, app.Namespace, kubeContext)
}

func printExistingAppOfApps(out io.Writer, app ArgoCDAppInfo) {
	fmt.Fprintf(out, "\n  Existing App of Apps:\n")
	fmt.Fprintf(out, "    Application:  %s (namespace %s)\n", app.Name, app.Namespace)
	if app.RepoURL != "" {
		fmt.Fprintf(out, "    Repository:   %s\n", app.RepoURL)
	}
	if app.TargetRevision != "" {
		fmt.Fprintf(out, "    Revision:     %s\n", app.TargetRevision)
	}
	if app.Path != "" {
		fmt.Fprintf(out, "    Path:         %s\n", app.Path)
	}
	if app.SyncStatus != "" || app.HealthStatus != "" {
		fmt.Fprintf(out, "    Sync/Health:  %s / %s\n",
			orUnknown(app.SyncStatus), orUnknown(app.HealthStatus))
	}
	fmt.Fprintln(out)
}

func orUnknown(value string) string {
	if value == "" {
		return "Unknown"
	}
	return value
}

// isInteractiveTerminal reports whether stdout is attached to a terminal, so
// non-interactive runs (CI, piped output) are not delayed by the countdown.
func isInteractiveTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd())) // #nosec G115
}
