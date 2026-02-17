package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var (
	orgFlag     string
	repoFlag    string
	appPathFlag string
	dryRunFlag  bool
	forceFlag   bool
)

// Current default values (detected from template)
const (
	defaultOrg      = "user-cube"
	defaultRepo     = "cluster-bootstrap"
	defaultAppPath  = "apps"
	defaultGoModule = "github.com/user-cube/cluster-bootstrap"
)

var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Template customization utilities",
	Long:  `Commands for customizing the cluster-bootstrap template with your own organization, repository, and configuration.`,
}

var customizeCmd = &cobra.Command{
	Use:   "customize",
	Short: "Customize the template with your organization and repository",
	Long: `Replace template placeholders (organization, repository, app-path) throughout the codebase.

This command updates:
- Repository URLs in apps/values.yaml and component values
- GitHub badges and links in README.md and documentation
- Go module paths in go.mod and import statements
- Documentation examples

Example:
  ./cli/cluster-bootstrap template customize --org mycompany --repo k8s-gitops
  ./cli/cluster-bootstrap template customize --org mycompany --repo k8s-gitops --app-path kubernetes/apps
  ./cli/cluster-bootstrap template customize --org mycompany --repo k8s-gitops --dry-run
`,
	RunE: runCustomize,
}

func init() {
	customizeCmd.Flags().StringVar(&orgFlag, "org", "", "GitHub organization or user name (required)")
	customizeCmd.Flags().StringVar(&repoFlag, "repo", "", "Repository name (required)")
	customizeCmd.Flags().StringVar(&appPathFlag, "app-path", defaultAppPath, "App of Apps path inside the repository")
	customizeCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Show what would be changed without modifying files")
	customizeCmd.Flags().BoolVar(&forceFlag, "force", false, "Skip confirmation prompt")
	_ = customizeCmd.MarkFlagRequired("org")  //#nosec G104 -- error only occurs if flag doesn't exist, which is impossible here
	_ = customizeCmd.MarkFlagRequired("repo") //#nosec G104 -- error only occurs if flag doesn't exist, which is impossible here

	templateCmd.AddCommand(customizeCmd)
	rootCmd.AddCommand(templateCmd)
}

func runCustomize(cmd *cobra.Command, args []string) error {
	// Get workspace root (parent of cli/)
	workspaceRoot, err := getWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to detect workspace root: %w", err)
	}

	if verbose {
		fmt.Println("🔧 Starting template customization")
		fmt.Printf("   Workspace root: %s\n", workspaceRoot)
	}

	// Validate inputs
	if err := validateInputs(); err != nil {
		return err
	}

	// Detect current values from go.mod
	currentOrg, currentRepo, err := detectCurrentValues(workspaceRoot)
	if err != nil {
		if verbose {
			fmt.Printf("⚠️  Could not detect current values: %v. Using defaults.\n", err)
		}
		currentOrg = defaultOrg
		currentRepo = defaultRepo
	}

	// Detect current app path from apps/values.yaml
	currentAppPath, err := detectCurrentAppPath(workspaceRoot)
	if err != nil {
		if verbose {
			fmt.Printf("⚠️  Could not detect current app path: %v. Using default.\n", err)
		}
		currentAppPath = defaultAppPath
	}

	// Detect current CLI module suffix (e.g., "cli" or "cluster-bootstrap-cli")
	currentModuleSuffix, err := detectCurrentModuleSuffix(workspaceRoot)
	if err != nil {
		return fmt.Errorf("failed to detect CLI module suffix: %w", err)
	}

	// Show summary
	fmt.Println("\n📝 Template Customization Summary")
	fmt.Println("═══════════════════════════════════")
	fmt.Printf("Organization:  %s → %s\n", currentOrg, orgFlag)
	fmt.Printf("Repository:    %s → %s\n", currentRepo, repoFlag)
	fmt.Printf("App Path:      %s → %s\n", currentAppPath, appPathFlag)
	fmt.Printf("Go Module:     github.com/%s/%s → github.com/%s/%s\n",
		currentOrg, currentRepo, orgFlag, repoFlag)

	if dryRunFlag {
		fmt.Println("\n🔍 DRY RUN MODE - No files will be modified")
	}

	// Confirm unless --force or --dry-run
	if !forceFlag && !dryRunFlag {
		fmt.Print("\nProceed with customization? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("❌ Customization cancelled")
			return nil
		}
	}

	fmt.Println()

	// Define replacements
	replacements := []replacement{
		{
			name:    "Repository URLs in apps/values.yaml",
			pattern: fmt.Sprintf("git@github.com:%s/%s.git", currentOrg, currentRepo),
			replace: fmt.Sprintf("git@github.com:%s/%s.git", orgFlag, repoFlag),
			files:   []string{"apps/values.yaml"},
		},
		{
			name:    "Repository URLs in component values",
			pattern: fmt.Sprintf("git@github.com:%s/%s.git", currentOrg, currentRepo),
			replace: fmt.Sprintf("git@github.com:%s/%s.git", orgFlag, repoFlag),
			files: []string{
				"components/argocd-repo-secret/values/dev.yaml",
				"components/argocd-repo-secret/values/staging.yaml",
				"components/argocd-repo-secret/values/prod.yaml",
			},
		},
		{
			name:    "GitHub badge URLs in README",
			pattern: fmt.Sprintf("github.com/%s/%s", currentOrg, currentRepo),
			replace: fmt.Sprintf("github.com/%s/%s", orgFlag, repoFlag),
			files:   []string{"README.md"},
		},
		{
			name:    "GitHub Pages documentation link",
			pattern: fmt.Sprintf("%s.github.io/%s", currentOrg, currentRepo),
			replace: fmt.Sprintf("%s.github.io/%s", orgFlag, repoFlag),
			files:   []string{"README.md"},
		},
		{
			name:    "Go module path in go.mod",
			pattern: fmt.Sprintf("module github.com/%s/%s/%s", currentOrg, currentRepo, currentModuleSuffix),
			replace: fmt.Sprintf("module github.com/%s/%s/%s", orgFlag, repoFlag, currentModuleSuffix),
			files:   []string{"cli/go.mod"},
		},
		{
			name:    "Go import statements",
			pattern: fmt.Sprintf(`"github.com/%s/%s/%s/`, currentOrg, currentRepo, currentModuleSuffix),
			replace: fmt.Sprintf(`"github.com/%s/%s/%s/`, orgFlag, repoFlag, currentModuleSuffix),
			files:   listGoFiles(workspaceRoot),
		},
		{
			name:    "App path in values",
			pattern: fmt.Sprintf("path: %s", currentAppPath),
			replace: fmt.Sprintf("path: %s", appPathFlag),
			files:   []string{"apps/values.yaml"},
		},
	}

	// Apply replacements
	changedFiles := make(map[string]int)
	for _, r := range replacements {
		if verbose {
			fmt.Printf("🔄 Processing: %s\n", r.name)
		}
		count, err := applyReplacement(workspaceRoot, r, dryRunFlag)
		if err != nil {
			fmt.Printf("❌ Failed: %v\n", err)
			return err
		}
		if count > 0 {
			fmt.Printf("   ✓ Updated %d file(s)\n", count)
			for _, f := range r.files {
				changedFiles[f]++
			}
		} else if verbose {
			fmt.Println("   • No changes needed")
		}
	}

	// Determine whether we need to run go mod tidy based on potentially changed Go files
	shouldRunGoModTidy := false
	for f := range changedFiles {
		if (strings.HasPrefix(f, "cli/") && strings.HasSuffix(f, ".go")) || f == "cli/go.mod" {
			shouldRunGoModTidy = true
			break
		}
	}

	// Run go mod tidy if relevant Go files under cli/ changed
	if !dryRunFlag && shouldRunGoModTidy {
		if verbose {
			fmt.Println("\n🔄 Running go mod tidy...")
		}
		if err := runGoModTidy(workspaceRoot); err != nil {
			fmt.Printf("⚠️  go mod tidy failed: %v\n", err)
			fmt.Println("   You may need to run it manually: cd cli && go mod tidy")
		} else if verbose {
			fmt.Println("   ✓ go mod tidy completed")
		}
	}

	// Summary
	fmt.Println("\n✅ Template customization complete!")
	if !dryRunFlag {
		fmt.Printf("Updated %d file(s)\n", len(changedFiles))
		fmt.Println("\nNext steps:")
		fmt.Println("  1. Review the changes: git diff")
		fmt.Println("  2. Test the CLI: cd cli && go test ./...")
		fmt.Printf("  3. Update your Git remote: git remote set-url origin git@github.com:%s/%s.git\n", orgFlag, repoFlag)
		fmt.Println("  4. Commit the changes: git add -A && git commit -m 'Customize template'")
	}

	return nil
}

type replacement struct {
	name    string   // Description for logging
	pattern string   // Text to find
	replace string   // Text to replace with
	files   []string // Files to process (relative to workspace root)
}

func applyReplacement(workspaceRoot string, r replacement, dryRun bool) (int, error) {
	changedCount := 0

	for _, relPath := range r.files {
		filePath := filepath.Join(workspaceRoot, relPath)

		// Check if file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			continue // Skip non-existent files
		}

		// Read file
		content, err := os.ReadFile(filePath) //#nosec G304 -- filePath is from predefined list in replacements struct, not user input
		if err != nil {
			return changedCount, fmt.Errorf("failed to read %s: %w", relPath, err)
		}

		originalContent := string(content)

		// Skip if pattern not found
		if !strings.Contains(originalContent, r.pattern) {
			continue
		}

		// Replace
		newContent := strings.ReplaceAll(originalContent, r.pattern, r.replace)

		// Skip if content unchanged (idempotence)
		if newContent == originalContent {
			continue
		}

		if dryRun {
			fmt.Printf("  [DRY RUN] Would update: %s\n", relPath)
			changedCount++
			continue
		}

		// Determine file permissions: preserve existing if possible, else default to 0644
		perm := os.FileMode(0644)
		if fi, err := os.Stat(filePath); err == nil {
			perm = fi.Mode().Perm()
		}

		// Write back
		if err := os.WriteFile(filePath, []byte(newContent), perm); err != nil {
			return changedCount, fmt.Errorf("failed to write %s: %w", relPath, err)
		}

		changedCount++
	}

	return changedCount, nil
}

func validateInputs() error {
	// Validate organization name
	validName := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$`)
	if !validName.MatchString(orgFlag) {
		return fmt.Errorf("invalid organization name '%s': must contain only alphanumeric characters and hyphens", orgFlag)
	}

	// Validate repository name
	if !validName.MatchString(repoFlag) {
		return fmt.Errorf("invalid repository name '%s': must contain only alphanumeric characters and hyphens", repoFlag)
	}

	// Validate app path (allow slashes)
	if strings.TrimSpace(appPathFlag) == "" {
		return fmt.Errorf("app-path cannot be empty")
	}

	return nil
}

func detectCurrentValues(workspaceRoot string) (org, repo string, err error) {
	goModPath := filepath.Join(workspaceRoot, "cli", "go.mod")
	content, err := os.ReadFile(goModPath) //#nosec G304 -- path is constructed from detected workspace root, not user input
	if err != nil {
		return "", "", err
	}

	// Parse "module github.com/org/repo/..." (e.g., /cli or /cluster-bootstrap-cli)
	re := regexp.MustCompile(`module github\.com/([^/]+)/([^/]+)/`)
	matches := re.FindStringSubmatch(string(content))
	if len(matches) < 3 {
		return "", "", fmt.Errorf("could not parse module path from go.mod")
	}

	return matches[1], matches[2], nil
}

func detectCurrentModuleSuffix(workspaceRoot string) (string, error) {
	goModPath := filepath.Join(workspaceRoot, "cli", "go.mod")
	content, err := os.ReadFile(goModPath) //#nosec G304 -- path is constructed from detected workspace root, not user input
	if err != nil {
		return "", err
	}

	// Parse "module github.com/org/repo/suffix" to extract the suffix
	re := regexp.MustCompile(`module github\.com/[^/]+/[^/]+/(.+)`)
	matches := re.FindStringSubmatch(string(content))
	if len(matches) < 2 {
		return "cli", nil // Default to "cli" for backward compatibility
	}

	return strings.TrimSpace(matches[1]), nil
}

func detectCurrentAppPath(workspaceRoot string) (string, error) {
	// Read the current app path from apps/values.yaml
	valuesPath := filepath.Join(workspaceRoot, "apps", "values.yaml")
	content, err := os.ReadFile(valuesPath) //#nosec G304 -- path is constructed from workspace root, not user input
	if err != nil {
		return "", fmt.Errorf("failed to read apps/values.yaml: %w", err)
	}

	// Parse the path value using regex to handle YAML structure variations
	// Matches: path: <value> (with optional indentation)
	re := regexp.MustCompile(`^\s*path:\s*(.+)$`)
	for _, line := range strings.Split(string(content), "\n") {
		if matches := re.FindStringSubmatch(line); matches != nil {
			return strings.TrimSpace(matches[1]), nil
		}
	}

	return "", fmt.Errorf("path field not found in apps/values.yaml")
}

func getWorkspaceRoot() (string, error) {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// If we're in cli/ directory, go up one level
	if filepath.Base(cwd) == "cli" {
		cwd = filepath.Dir(cwd)
	}

	// Check if we're at workspace root (has apps/ and cli/ directories)
	appsPath := filepath.Join(cwd, "apps")
	cliPath := filepath.Join(cwd, "cli")
	if _, err := os.Stat(appsPath); err == nil {
		if _, err := os.Stat(cliPath); err == nil {
			// Resolve symlinks for consistent path comparison (macOS /var -> /private/var)
			resolvedPath, err := filepath.EvalSymlinks(cwd)
			if err != nil {
				return cwd, nil // fallback to unresolved path
			}
			return resolvedPath, nil
		}
	}

	return "", fmt.Errorf("workspace root not detected. Run from workspace root or cli/ directory")
}

func listGoFiles(workspaceRoot string) []string {
	var goFiles []string

	cliRoot := filepath.Join(workspaceRoot, "cli")
	_ = filepath.Walk(cliRoot, func(path string, info os.FileInfo, err error) error { //#nosec G104 -- walk error is intentionally ignored to collect as many files as possible
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			// Convert to relative path from workspace root
			relPath, _ := filepath.Rel(workspaceRoot, path)
			goFiles = append(goFiles, relPath)
		}
		return nil
	})

	return goFiles
}

func runGoModTidy(workspaceRoot string) error {
	cliDir := filepath.Join(workspaceRoot, "cli")
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = cliDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(output))
	}
	return nil
}
