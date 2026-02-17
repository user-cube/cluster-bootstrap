# Troubleshooting Guide

Common issues and solutions when using the cluster-bootstrap CLI.

## Prerequisites

### kubectl not found

**Error:** `kubectl not found or not accessible`

**Solution:**
1. Install kubectl: https://kubernetes.io/docs/tasks/tools/
2. Verify installation: `kubectl version --client`
3. Ensure it's in your PATH: `which kubectl`

### Helm not found

**Error:** `helm not found or not accessible`

**Solution:**
1. Install Helm: https://helm.sh/docs/intro/install/
2. Verify installation: `helm version`

### SOPS not found

**Error:** `sops not found or not accessible` (when using SOPS encryption)

**Solution:**
1. Install SOPS: `brew install sops` or from https://github.com/mozilla/sops
2. Verify installation: `sops --version`

### Age not found

**Error:** `age not found or not accessible` (when using age encryption)

**Solution:**
1. Install age: `brew install age` or from https://github.com/FiloSottile/age
2. Verify installation: `age-keygen --version`

### Git-crypt not found

**Error:** `git-crypt not found or not accessible` (when using git-crypt encryption)

**Solution:**
1. Install git-crypt: `brew install git-crypt` or from https://github.com/AGWA/git-crypt
2. Verify installation: `git-crypt --version`

## Kubeconfig & Cluster Connection

### Cannot connect to cluster

**Error:** `failed to connect to cluster: connection refused`

**Solution:**
1. Verify cluster is running: `kubectl cluster-info`
2. Check current context: `kubectl config current-context`
3. Verify kubeconfig: `kubectl config view`
4. Try explicitly setting kubeconfig:
   ```bash
   ./cli/cluster-bootstrap bootstrap dev --kubeconfig ~/.kube/config --context my-cluster
   ```

### Context not found

**Error:** `context nonexistent-context not found in kubeconfig`

**Solution:**
1. List available contexts: `kubectl config get-contexts`
2. Use correct context name:
   ```bash
   ./cli/cluster-bootstrap bootstrap dev --context docker-desktop
   ```

### Invalid kubeconfig file

**Error:** `failed to load kubeconfig /path/to/config: permission denied`

**Solution:**
1. Check file permissions: `ls -la ~/.kube/config`
2. Should be readable by owner: `chmod 600 ~/.kube/config`
3. Verify path exists and is accessible

## Permissions & RBAC

### Permission denied when creating resources

**Error:** `permission denied: cannot create resource "pods"...`

**Solution:**
1. Check your current user: `kubectl auth whoami`
2. Check available permissions:
   ```bash
   kubectl auth can-i create pods
   kubectl auth can-i create namespaces
   ```
3. Your cluster role needs:
   - Create namespaces
   - Create/update secrets in the `argocd` namespace
   - Create/update applications (if using ArgoCD CRDs)
4. Ask your cluster administrator for appropriate permissions

### Secret file permissions

**Error:** `file permissions too permissive for secret: ...mode: 644 (should be 600)`

**Solution:**
1. Fix file permissions:
   ```bash
   chmod 600 ~/.age/key.txt
   chmod 600 ./git-crypt-key
   ```
2. All secret files should only be readable by the owner

## Secrets & Encryption

### Secrets file not found

**Error:** `secrets-file does not exist: secrets.dev.enc.yaml`

**Solution:**
1. Create the secrets file first:
   ```bash
   ./cli/cluster-bootstrap init --provider sops
   ```
2. Or explicitly provide the file:
   ```bash
   ./cli/cluster-bootstrap bootstrap dev --secrets-file ./custom-secrets.enc.yaml
   ```

### Invalid secrets file format

**Error:** `failed to parse secrets file: invalid YAML`

**Solution:**
1. Check file syntax: `cat secrets.dev.enc.yaml | head` (for git-crypt)
2. For SOPS: ensure it's properly encrypted/decrypted
3. Validate YAML: `kubectl apply --dry-run=client -f secrets.dev.yaml`

### Cannot decrypt secrets

**Error:** `failed to decrypt secrets: age: decryption failed`

**Solution - Age:**
1. Verify age key file exists: `ls ~/.age/key.txt`
2. Set age key path explicitly:
   ```bash
   ./cli/cluster-bootstrap bootstrap dev --age-key-file ~/.age/key.txt
   ```
3. Or set environment variable:
   ```bash
   export SOPS_AGE_KEY_FILE=~/.age/key.txt
   ```

**Solution - Git-crypt:**
1. Verify git-crypt is initialized: `git-crypt status`
2. Ensure git-crypt key is present: `git-crypt unlock`
3. Check .gitattributes has pattern: `grep secrets .gitattributes`

## Helm Installation

### Helm install/upgrade timeout

**Error:** `Helm install timed out: context deadline exceeded`

**Solution:**
1. Increase timeout (currently 5 min try increasing resources):
   ```bash
   kubectl get nodes # check node resources
   kubectl top nodes # check current usage
   ```
2. Check pod status during install:
   ```bash
   kubectl get pods -n argocd -w
   kubectl describe pod <pod-name> -n argocd
   ```
3. Check logs:
   ```bash
   kubectl logs <pod-name> -n argocd --tail=50
   ```

### Image pull failed

**Error:** `ImagePullBackOff: failed to pull image`

**Solution:**
1. Verify image is accessible:
   ```bash
   docker pull quay.io/argoproj/argocd:v2.x.x
   ```
2. Check image pull secrets (if using private registry):
   ```bash
   kubectl get secrets -n argocd
   ```
3. Check node logs:
   ```bash
   kubectl describe nodes
   ```
4. Add image pull secret to argocd values

### Release already exists

**Error:** `release argocd already exists. Use --force to force`

**Solution:**
1. ArgoCD is already installed. Skip installation:
   ```bash
   ./cli/cluster-bootstrap bootstrap dev --skip-argocd-install
   ```
2. Or uninstall existing release:
   ```bash
   helm uninstall argocd -n argocd
   ```

## ArgoCD & Application

### Application CRD not found

**Error:** `ArgoCD CRD not found: ApplicationCRD not found`

**Solution:**
1. Verify ArgoCD is installed:
   ```bash
   helm list -n argocd
   kubectl get crd applications.argoproj.io
   ```
2. Install ArgoCD if missing:
   ```bash
   ./cli/cluster-bootstrap bootstrap dev
   ```
3. Wait for CRDs to be created:
   ```bash
   kubectl wait --for=condition=established crd/applications.argoproj.io
   ```

### App of Apps not syncing

**Issue:** Application created but not syncing

**Solution:**
1. Check application status:
   ```bash
   kubectl get application -n argocd -o wide
   kubectl describe application app-of-apps -n argocd
   ```
2. Check ArgoCD server logs:
   ```bash
   kubectl logs -f <argocd-server-pod> -n argocd
   ```
3. Verify Git repository access:
   - SSH key secret: `kubectl get secret repo-ssh-key -n argocd -o yaml`
   - Test SSH: `ssh -T git@github.com`
4. Check app repository URL: `kubectl get secret repo-ssh-key -n argocd -o jsonpath='{.data.url}' | base64 -d`

## Debugging

### Enable verbose output

**Command:**
```bash
./cli/cluster-bootstrap -v bootstrap dev
```

This enables:
- Configuration details
- Stage timings
- Resource creation details (without exposing secrets)
- Detailed error messages

### Dry-run mode

**Command:**
```bash
./cli/cluster-bootstrap bootstrap dev --dry-run
```

This shows manifests that would be created without applying them.

**Output to file:**
```bash
./cli/cluster-bootstrap bootstrap dev --dry-run --dry-run-output /tmp/bootstrap.json
```

### Check cluster connectivity

```bash
# Connection test
kubectl cluster-info
kubectl get nodes
kubectl auth whoami

# RBAC test
kubectl auth can-i create namespaces
kubectl auth can-i create secrets -n argocd
```

### Check ArgoCD status

```bash
# ArgoCD components
kubectl get pods -n argocd
kubectl rollout status deploy/argocd-server -n argocd
kubectl rollout status statefulset/argocd-application-controller -n argocd

# ArgoCD UI access
kubectl port-forward svc/argocd-server -n argocd 8080:443
# Then visit https://localhost:8080
```

## Common Patterns

### Bootstrap with custom app path

If your apps are in a different directory:

```bash
./cli/cluster-bootstrap bootstrap dev --app-path k8s/apps
```

Or with auto-detection (if Chart.yaml + templates/application.yaml exist):
```bash
./cli/cluster-bootstrap bootstrap dev
```

### Bootstrap with repo in subdirectory

```bash
./cli/cluster-bootstrap --base-dir ./k8s bootstrap dev
```

### Bootstrap with specific git-crypt key

```bash
./cli/cluster-bootstrap bootstrap dev \
  --encryption git-crypt \
  --gitcrypt-key-file ./git-crypt-key
```

### Store Vault token after bootstrap

```bash
./cli/cluster-bootstrap vault-token --token <root-token>
# Or via stdin
echo "<root-token>" | ./cli/cluster-bootstrap vault-token
# Or via prompt
./cli/cluster-bootstrap vault-token
```

## Getting Help

1. **Check verbose output**: Use `-v` flag for detailed information
2. **Check logs**: `kubectl logs` for component logs
3. **Dry-run**: Use `--dry-run` to see what would be created
4. **Test connectivity**: Verify `kubectl cluster-info` and `kubectl auth whoami`
5. **Review configuration**: Check `.sops.yaml` and values files

## Report Issues

If you encounter an issue:

1. Reproduce with verbose output: `./cli/cluster-bootstrap -v bootstrap dev`
2. Collect dry-run output: `./cli/cluster-bootstrap bootstrap dev --dry-run --dry-run-output /tmp/debug.json`
3. Check cluster resources: `kubectl get all -n argocd`
4. Share logs without exposing secrets
5. Report to: https://github.com/user-cube/cluster-bootstrap/issues
