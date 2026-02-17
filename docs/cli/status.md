# status

```bash
./cli/cluster-bootstrap status <environment>
```

Shows cluster status and component information. This is an alias of the `info` command.

## What it does

1. Connects to the cluster using the provided kubeconfig/context
2. Reports cluster version and core component readiness
3. Lists component versions and replica counts
4. Auto-discovers and reports ArgoCD Applications (sync/health status)
5. Optionally runs health checks

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--kubeconfig` | `~/.kube/config` | Path to kubeconfig file |
| `--context` | current context | Kubeconfig context to use |
| `--wait-for-health` | `false` | Include health check results |
| `--health-timeout` | `180` | Timeout in seconds for health checks |

## Examples

```bash
# Basic status
./cli/cluster-bootstrap status dev

# Include health checks
./cli/cluster-bootstrap status dev --wait-for-health

# Use a specific kubeconfig and context
./cli/cluster-bootstrap status dev --kubeconfig ~/.kube/my-config --context my-cluster
```
