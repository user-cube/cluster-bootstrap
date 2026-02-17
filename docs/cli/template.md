# Template Customize

The `template customize` command allows you to personalize the cluster-bootstrap-cli template with your own organization, repository name, and configuration paths.

## Usage

```bash
./cli/cluster-bootstrap-cli template customize --org <organization> --repo <repository> [flags]
```

## What It Does

This command replaces template placeholders throughout the codebase:

- **Repository URLs**: Updates Git repository URLs in `apps/values.yaml` and component values files
- **GitHub references**: Updates GitHub badges, links, and GitHub Pages URLs in documentation
- **Go module paths**: Updates the module path in `go.mod` and all import statements in Go files

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--org` | — | GitHub organization or user name (required) |
| `--repo` | — | Repository name (required) |
| `--app-path` | `apps` | App of Apps path (runtime default, not stored in config) |
| `--dry-run` | `false` | Preview changes without modifying files |
| `--force` | `false` | Skip confirmation prompt |

## Examples

### Basic Customization

Replace the default `user-cube/cluster-bootstrap` with your organization and repository:

```bash
./cli/cluster-bootstrap-cli template customize --org mycompany --repo k8s-gitops
```

### Custom App Path

If your App of Apps lives in a different directory:

```bash
./cli/cluster-bootstrap-cli template customize \
  --org mycompany \
  --repo k8s-gitops \
  --app-path kubernetes/apps
```

### Dry Run

Preview what would be changed without modifying files:

```bash
./cli/cluster-bootstrap-cli template customize \
  --org mycompany \
  --repo k8s-gitops \
  --dry-run
```

### Force (Skip Confirmation)

Skip the interactive confirmation prompt:

```bash
./cli/cluster-bootstrap-cli template customize \
  --org mycompany \
  --repo k8s-gitops \
  --force
```

## What Gets Updated

The command modifies the following files:

### Repository Configuration
- `apps/values.yaml` - Main App of Apps repository URL
- `components/argocd-repo-secret/values/dev.yaml` - Development environment repo URL
- `components/argocd-repo-secret/values/staging.yaml` - Staging environment repo URL
- `components/argocd-repo-secret/values/prod.yaml` - Production environment repo URL

### Documentation
- `README.md` - GitHub badges and documentation links
- Other documentation files with GitHub references

### Go Code
- `cli/go.mod` - Go module path
- All `.go` files in `cli/` - Import statements

> **Note**: The `--app-path` flag is used at runtime by the `bootstrap` command and is not stored in repository configuration files.

## Workflow

1. **Detection**: The command detects current values from `cli/go.mod`
2. **Validation**: Validates organization and repository names (alphanumeric and hyphens only)
3. **Summary**: Displays a summary of changes to be made
4. **Confirmation**: Prompts for confirmation (unless `--dry-run` or `--force`)
5. **Replacement**: Applies all replacements across the codebase
6. **Go Modules**: Automatically runs `go mod tidy` to update module dependencies
7. **Summary**: Displays completion summary with next steps

## Example Output

```
📝 Template Customization Summary
═══════════════════════════════════
Organization:  user-cube → mycompany
Repository:    cluster-bootstrap → k8s-gitops
App Path:      apps
Go Module:     github.com/user-cube/cluster-bootstrap → github.com/mycompany/k8s-gitops

Proceed with customization? [y/N]: y

🔄 Processing: Repository URLs in apps/values.yaml
   ✓ Updated 1 file(s)
🔄 Processing: Repository URLs in component values
   ✓ Updated 3 file(s)
🔄 Processing: GitHub badge URLs in README
   ✓ Updated 1 file(s)
🔄 Processing: GitHub Pages documentation link
   ✓ Updated 1 file(s)
🔄 Processing: Go module path in go.mod
   ✓ Updated 1 file(s)
🔄 Processing: Go import statements
   ✓ Updated 15 file(s)

🔄 Running go mod tidy...
   ✓ go mod tidy completed

✅ Template customization complete!
Updated 22 file(s)

Next steps:
  1. Review the changes: git diff
  2. Test the CLI: cd cli && go test ./...
  3. Update your Git remote: git remote set-url origin git@github.com:mycompany/k8s-gitops.git
  4. Commit the changes: git add -A && git commit -m 'Customize template'
```

## Validation Rules

### Organization Name
- Must contain only alphanumeric characters and hyphens
- Cannot start or end with a hyphen
- Examples: `mycompany`, `my-company`, `company123`

### Repository Name
- Must contain only alphanumeric characters and hyphens
- Cannot start or end with a hyphen
- Examples: `k8s-gitops`, `cluster-config`, `infrastructure-repo`

### App Path
- Cannot be empty
- Can contain forward slashes for nested paths
- Examples: `apps`, `kubernetes/apps`, `k8s/argocd/apps`

## Idempotence

The command is idempotent - it detects if the template is already customized with the provided values:

```bash
$ ./cli/cluster-bootstrap-cli template customize --org mycompany --repo k8s-gitops
✅ Template already customized with org=mycompany repo=k8s-gitops
   Use --force to re-apply customization
```

Use `--force` to re-apply the same customization if needed.

## Running from Different Directories

The command automatically detects the workspace root:

- **From workspace root**: Detects `apps/` and `cli/` directories
- **From `cli/` directory**: Goes up one level to find workspace root
- **From other locations**: Returns an error

## Post-Customization Steps

After customizing the template:

1. **Review changes**: Use `git diff` to verify all replacements
2. **Test the CLI**: Run the test suite to ensure everything works
   ```bash
   cd cli && go test ./...
   ```
3. **Update Git remote**: Point to your new repository
   ```bash
   git remote set-url origin git@github.com:mycompany/k8s-gitops.git
   ```
4. **Commit changes**: Save the customization
   ```bash
   git add -A
   git commit -m 'Customize template for mycompany/k8s-gitops'
   ```
5. **Push to your repository**:
   ```bash
   git push -u origin main
   ```

## Troubleshooting

### "workspace root not detected"

Make sure you're running the command from either:
- The workspace root (where `apps/` and `cli/` directories exist)
- The `cli/` directory

### "go mod tidy failed"

If `go mod tidy` fails after customization:
```bash
cd cli
go mod tidy
```

This usually happens if there are network issues or Go version incompatibilities.

### Incomplete customization

If some files are not updated, ensure:
- All files are writable (not read-only)
- The workspace is not in a protected location
- File paths match the expected structure

Run with `--dry-run` first to preview changes.
