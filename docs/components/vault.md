# Vault

**Namespace:** `vault` | **Sync Wave:** 1 | **Chart:** `vault` v0.29.1

HashiCorp Vault provides secrets management. It stores sensitive data like SSH keys and credentials that are consumed by the External Secrets Operator.

## Upstream Chart

- **Chart:** `vault`
- **Version:** 0.29.1
- **Repository:** `https://helm.releases.hashicorp.com`

## Key Configuration

**Base values:**

- UI enabled
- Injector disabled (secrets are consumed via External Secrets, not sidecar injection)

**Per-environment:**

| Setting | Dev | Staging | Prod |
|---------|-----|---------|------|
| Mode | Dev (in-memory) | Standalone | HA (Raft) |
| Persistent storage | None | 10Gi | 10Gi |
| Auto-unseal | N/A | No | Yes |
| Replicas | 1 | 1 | 3 (HA) |

## Custom Templates

Vault includes custom Job templates beyond the upstream chart:

- **`vault-auto-unseal-job.yaml`** — initializes and unseals Vault in production (HA mode)
- **`vault-auto-unseal-rbac.yaml`** — RBAC for auto-unseal operations
- **`vault-config-job.yaml`** — configures the Kubernetes auth backend in Vault
- **`vault-seed-job.yaml`** — seeds the SSH private key from a Kubernetes Secret into Vault's KV store
- **`vault-seed-rbac.yaml`** — RBAC for seed operations

## Environment Details

### Dev

Dev mode runs Vault in-memory with no persistence. Vault is pre-unsealed and ready to use immediately — ideal for local development.

### Staging

Standalone mode with persistent storage. Vault must be manually initialized and unsealed after first deploy.

### Production

HA mode with Raft storage backend. Auto-unseal is enabled via the auto-unseal Job. After `vault operator init`, store the root token:

```bash
./cli/cluster-bootstrap vault-token --token <root-token>
echo "<root-token>" | ./cli/cluster-bootstrap vault-token
./cli/cluster-bootstrap vault-token
```

## Files

```
components/vault/
├── Chart.yaml
├── templates/
│   ├── vault-auto-unseal-job.yaml
│   ├── vault-auto-unseal-rbac.yaml
│   ├── vault-config-job.yaml
│   ├── vault-seed-job.yaml
│   └── vault-seed-rbac.yaml
└── values/
    ├── base.yaml
    ├── dev.yaml
    ├── staging.yaml
    └── prod.yaml
```
