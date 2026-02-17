# Using Cluster Bootstrap in a Subdirectory

This guide explains how to configure cluster-bootstrap when your Kubernetes manifests are located in a subdirectory of your Git repository (e.g., `/k8s/`, `/infrastructure/`, etc.).

## The Problem

By default, cluster-bootstrap expects the repository structure to be at the root:

```
repo/
├── apps/
├── components/
├── cli/
└── ...
```

If your structure is instead:

```
repo/
└── k8s/
    ├── apps/
    ├── components/
    └── ...
```

ArgoCD will fail with errors like:

```
ComparisonError: Failed to load target state: failed to generate manifest for source 1 of 1:
rpc error: code = Unknown desc = failed to list refs: repository not found
```

This happens because ArgoCD tries to access paths like `components/argocd` when they're actually at `k8s/components/argocd`.

## The Solution

You need to configure **three things** to make it work:

### 1. Update `apps/values.yaml`

Add the `basePath` field to tell ArgoCD where the components are located:

```yaml
environment: dev

repo:
  url: git@github.com:yourorg/yourrepo.git
  targetRevision: main
  basePath: "k8s"  # 👈 Add this line with your subdirectory name

components:
  argocd:
    enabled: true
    namespace: argocd
    syncWave: "0"
    syncOptions:
      - ServerSideApply=true
  # ... rest of your components
```

### 2. Use `--base-dir` Flag

When running CLI commands, use the `--base-dir` flag to point to your subdirectory:

```bash
./cluster-bootstrap-cli/cluster-bootstrap-cli --base-dir ./k8s bootstrap dev
```

This tells the CLI where to find:
- Chart.yaml files
- Values files
- Secrets files
- Component definitions

### 3. Use `--app-path` Flag (if needed)

For bootstrap, specify the full path to the apps directory:

```bash
./cluster-bootstrap-cli/cluster-bootstrap-cli --base-dir ./k8s bootstrap dev --app-path k8s/apps
```

## Complete Example

Here's a complete example for a repository with manifests in the `k8s/` subdirectory:

### 1. Repository Structure

```
my-repo/
├── README.md
├── src/              # Your application code
└── k8s/              # Kubernetes manifests
    ├── apps/
    │   ├── Chart.yaml
    │   ├── values.yaml
    │   ├── values/
    │   │   ├── dev.yaml
    │   │   ├── staging.yaml
    │   │   └── prod.yaml
    │   └── templates/
    │       └── application.yaml
    ├── components/
    │   ├── argocd/
    │   ├── vault/
    │   └── ...
    └── secrets.dev.enc.yaml
```

### 2. Update Configuration

Edit `k8s/apps/values.yaml`:

```yaml
environment: dev

repo:
  url: git@github.com:myorg/my-repo.git
  targetRevision: main
  basePath: "k8s"      # 👈 Add this

components:
  argocd:
    enabled: true
    namespace: argocd
    syncWave: "0"
    syncOptions:
      - ServerSideApply=true

  vault:
    enabled: true
    namespace: vault
    syncWave: "1"

  external-secrets:
    enabled: true
    namespace: external-secrets
    syncWave: "1"
    syncOptions:
      - ServerSideApply=true

  # ... other components
```

### 3. Initialize Secrets

```bash
./cluster-bootstrap-cli/cluster-bootstrap-cli --base-dir ./k8s init --provider sops
```

This creates `k8s/secrets.dev.enc.yaml` with the template.

### 4. Edit and Encrypt Secrets

```bash
# Edit the secrets file
sops k8s/secrets.dev.enc.yaml

# Add your Git repository SSH key and other secrets
```

### 5. Bootstrap the Cluster

The CLI automatically detects if you're running from a subdirectory and adjusts paths accordingly. You can use either method:

#### Option A: From repository root with --base-dir

```bash
cd /path/to/my-repo
./k8s/cli/cluster-bootstrap --base-dir ./k8s bootstrap dev \
  --app-path k8s/apps \
  --wait-for-health -v
```

#### Option B: From subdirectory with relative path

```bash
cd /path/to/my-repo/k8s
./cluster-bootstrap-cli/cluster-bootstrap-cli bootstrap dev \
  --app-path apps \
  --wait-for-health -v
```

#### Option C: From subdirectory with full path

```bash
cd /path/to/my-repo/k8s
./cluster-bootstrap-cli/cluster-bootstrap-cli bootstrap dev \
  --app-path k8s/apps \
  --wait-for-health -v
```

**All three work!** The CLI is smart enough to:
- Detect you're in a Git subdirectory (`k8s/`)
- Strip the prefix when needed for local validation
- Always pass the correct full path to ArgoCD

The CLI will automatically find:
- `./age-key.txt` (or use `--age-key-file` to specify)
- `./secrets.dev.enc.yaml` (or use `--secrets-file` to specify)
- `./apps/` and `./components/`

### 6. Verify Components

After bootstrap, verify that ArgoCD applications have the correct paths:

```bash
# Check app-of-apps
kubectl get application app-of-apps -n argocd -o yaml | grep path:
# Should show: path: k8s/apps

# Check individual component applications
kubectl get applications -n argocd -o custom-columns=NAME:.metadata.name,PATH:.spec.source.path
# Should show paths like:
# argocd                      k8s/components/argocd
# vault                       k8s/components/vault
# external-secrets            k8s/components/external-secrets
```

## How It Works

The `basePath` field is used in the Helm template (`apps/templates/application.yaml`) to construct the correct path:

```yaml
# Without basePath (default):
path: components/{{ $name }}
# Result: components/argocd

# With basePath: "k8s":
path: {{ if $.Values.repo.basePath }}{{ $.Values.repo.basePath }}/{{ end }}components/{{ $name }}
# Result: k8s/components/argocd
```

## Troubleshooting

### Error: "repository not found"

**Symptom:** ArgoCD applications show `ComparisonError` with "repository not found"

**Solution:**
1. Verify `basePath` is set in `apps/values.yaml`
2. Ensure you used `--base-dir` flag when bootstrapping
3. Force refresh ArgoCD applications:
   ```bash
   kubectl patch application app-of-apps -n argocd --type merge \
     -p '{"metadata":{"annotations":{"argocd.argoproj.io/refresh":"hard"}}}'
   ```

### Error: "secrets file not found"

**Symptom:** CLI can't find secrets file

**Solution:**
Use the `--base-dir` flag:
```bash
./cluster-bootstrap-cli/cluster-bootstrap-cli --base-dir ./k8s bootstrap dev
```

### Error: "app path does not exist"

**Symptom:** `app-path does not exist: stat <path>: no such file or directory`

**Solution:**

The CLI now auto-detects Git subdirectories. Make sure you're using the correct relative path:

```bash
# ✅ From subdirectory - use relative path
cd /path/to/repo/k8s
./cluster-bootstrap-cli/cluster-bootstrap-cli bootstrap dev --app-path apps

# ✅ From root with --base-dir - use full path
cd /path/to/repo
./k8s/cli/cluster-bootstrap --base-dir ./k8s bootstrap dev --app-path k8s/apps
```

**How auto-detection works:**
- When you run from `k8s/` and specify `--app-path apps`
- The CLI detects you're in a Git subdirectory
- Automatically converts to `k8s/apps` for ArgoCD
- Validates locally using `./apps`

### Paths are still wrong after update

**Symptom:** Updated `basePath` but ArgoCD still uses old paths

**Solution:**
The app-of-apps needs to be refreshed/synced:

```bash
# Option 1: Hard refresh via annotation
kubectl patch application app-of-apps -n argocd --type merge \
  -p '{"metadata":{"annotations":{"argocd.argoproj.io/refresh":"hard"}}}'

# Option 2: Re-run bootstrap (idempotent)
./cluster-bootstrap-cli/cluster-bootstrap-cli --base-dir ./k8s bootstrap dev --app-path k8s/apps

# Option 3: Use ArgoCD UI
# Navigate to app-of-apps → Click "Refresh" → Select "Hard Refresh"
```

## Summary

To use cluster-bootstrap with manifests in a subdirectory:

1. ✅ Add `repo.basePath: "subdirectory"` to `apps/values.yaml`
2. ✅ Run from **anywhere** - the CLI auto-detects your location
3. ✅ Use `--app-path` relative to where you're running
4. ✅ Verify paths in ArgoCD applications after deployment

### Quick Reference

```bash
# ✅ Option 1: From repository root with --base-dir
cd /path/to/repo
./k8s/cli/cluster-bootstrap --base-dir ./k8s bootstrap dev --app-path k8s/apps

# ✅ Option 2: From subdirectory (auto-detected)
cd /path/to/repo/k8s
./cluster-bootstrap-cli/cluster-bootstrap-cli bootstrap dev --app-path apps
```

### How Auto-Detection Works

The CLI is smart enough to detect your location:
- **Finds Git root**: Walks up directories looking for `.git`
- **Calculates relative path**: Determines your position in the repo (e.g., `k8s/`)
- **Auto-adjusts paths**: Converts `apps` → `k8s/apps` for ArgoCD automatically
- **Validates locally**: Checks files exist relative to your current location

**No more path confusion!** Just use paths relative to where you are.

This configuration ensures ArgoCD can correctly locate all components in your repository subdirectory.
