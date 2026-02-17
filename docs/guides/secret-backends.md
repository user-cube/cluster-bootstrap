# Secret Backends Configuration

The `argocd-repo-secret` component supports multiple secret backends for managing the Git repository credentials.

## Available Backends

### 1. None (Default for no secrets backend)

**Backend:** `backend: none`

The bootstrap CLI creates the `repo-ssh-key` secret directly without any secrets backend.

**Use case:** Simple setups that don't need dynamic secret rotation or centralized secrets storage.

**Configuration:**
```yaml
# apps/values/dev-no-vault.yaml
argocd-repo-secret:
  enabled: true
  backend: none
```

**How it works:**
1. Bootstrap CLI decrypts secrets file (SOPS or git-crypt)
2. Bootstrap creates `repo-ssh-key` Secret in argocd namespace
3. ArgoCD uses the secret for Git repository access
4. Component deployment doesn't create any ExternalSecret

**Pros:**
- Simplest setup - no secrets backend required
- Fewer components to manage
- Faster bootstrap process

**Cons:**
- No dynamic secret rotation
- No centralized secrets storage
- Secret lifecycle tied to bootstrap cycles

---

### 2. Vault (Default)

**Backend:** `backend: vault`

Uses HashiCorp Vault as the central secrets store with External Secrets Operator synchronizing credentials into Kubernetes.

**Use case:** Enterprise setups requiring:
- Centralized secrets management
- Dynamic secret rotation
- Audit logging
- Complex access control policies

**Configuration:**
```yaml
# apps/values/dev.yaml (default)
argocd-repo-secret:
  enabled: true
  backend: vault

vault:
  address: http://vault.vault.svc:8200
  kvPath: secret
  kvVersion: "v2"
  kubernetesRole: external-secrets
```

**How it works:**
1. Bootstrap CLI stores SSH key in Vault
2. External Secrets Operator creates SecretStore pointing to Vault
3. ExternalSecret watches Vault and syncs into K8s
4. ArgoCD uses synced secret for Git access
5. Secret rotation happens transparently

**Pros:**
- Enterprise-grade secrets management
- Dynamic rotation support
- Audit logging
- Centralized policy enforcement

**Cons:**
- Requires Vault setup and maintenance
- Additional dependencies (External Secrets Operator)
- More complex troubleshooting

---

### 3. AWS Secrets Manager

**Backend:** `backend: aws-secrets`

Integrates with AWS Secrets Manager for organizations already using AWS.

**Use case:** AWS-first organizations that want to use native AWS managed services.

**Configuration:**
```yaml
# apps/values/dev-aws.yaml
argocd-repo-secret:
  enabled: true
  backend: aws-secrets

aws:
  region: eu-west-1
  authSecret: aws-credentials  # Pre-created K8s secret with AWS credentials
```

**How it works:**
1. Store SSH key in AWS Secrets Manager
2. External Secrets Operator creates SecretStore with AWS credentials
3. ExternalSecret syncs from AWS into Kubernetes
4. ArgoCD uses synced secret

**Pros:**
- Native AWS integration
- IAM policy enforcement
- CloudTrail audit logging
- Pay-per-secret pricing

**Cons:**
- AWS-specific solution
- Requires External Secrets Operator
- AWS credential management overhead

---

### 4. Sealed Secrets (Future)

**Backend:** `backend: sealed-secrets`

Uses Bitnami Sealed Secrets for local encryption without external dependencies.

**Status:** Planned support

**Use case:** Organizations wanting encryption without external secrets services.

---

## Comparison Matrix

| Feature | None | Vault | AWS Secrets |
|---------|------|-------|-------------|
| Setup Complexity | ★☆☆ | ★★★ | ★★☆ |
| External Dependencies | None | Vault Server | AWS Account |
| Dynamic Rotation | No | Yes | Yes |
| Audit Logging | Limited | Full | CloudTrail |
| Cost | Free | Self-hosted/managed | Per-secret |
| Fallback on Sync Failure | Static bootstrap value | Stops syncing | Stops syncing |
| Best For | Development | Enterprise | AWS organizations |

## Migration Path

**No Backend → Vault:**
```bash
# 1. Set up Vault and store SSH key
vault kv put secret/argocd/repo-ssh-key sshPrivateKey=@ssh-key

# 2. Update values
backend: vault

# 3. Deploy (ArgoCD syncs, ExternalSecret takes over)
```

**Vault → No Backend:**
```bash
# 1. Bootstrap with current backend (continues working)
./cluster-bootstrap bootstrap dev

# 2. Update values
backend: none

# 3. Deploy (removes ExternalSecret, keeps bootstrap secret)
# Note: Secret stays in Vault, just not synced
```

## Troubleshooting

### Secrets not syncing

**Check which backend is configured:**
```bash
helm get values components/argocd-repo-secret
```

**For Vault:**
```bash
# Verify ExternalSecret status
kubectl get externalsecret -n argocd
kubectl describe externalsecret repo-ssh-key -n argocd

# Check Vault connectivity
kubectl logs -n external-secrets -l app=external-secrets
```

**For AWS:**
```bash
# Verify AWS credentials secret
kubectl get secret aws-credentials -n argocd

# Check External Secrets logs
kubectl logs -n external-secrets -l app=external-secrets
```

**For None:**
```bash
# Verify secret was created by bootstrap
kubectl describe secret repo-ssh-key -n argocd
```

### Testing Backend Switch

```bash
# 1. Verify current setup works
./cluster-bootstrap bootstrap dev

# 2. Test new backend with dry-run
./cluster-bootstrap bootstrap dev --dry-run

# 3. Update values and apply
# (ArgoCD will reconcile automatically)

# 4. Verify secret still accessible
kubectl describe secret repo-ssh-key -n argocd
```
