# Cilium

**Namespace:** `kube-system` | **Lifecycle:** opt-in bootstrap, then ArgoCD-managed | **Chart:** `cilium` v1.20.1

Cilium is optional. It is installed only when `bootstrap` receives `--enable-cilium`:

```bash
cluster-bootstrap-cli bootstrap dev --enable-cilium
```

## Bootstrap and ownership handoff

1. The CLI installs or upgrades Helm release `cilium` in `kube-system`.
2. Helm waits for the chart workloads, including the Cilium DaemonSet, to become ready. ArgoCD is not installed if this step fails.
3. The CLI installs or upgrades ArgoCD.
4. The CLI creates or updates an ArgoCD Application named `cilium`, pointing at `components/cilium/` in the configured repository and target revision.
5. ArgoCD renders the same chart version, release name, namespace, base values, and environment values used by bootstrap.

This matching configuration lets ArgoCD take over the existing resources without a second release identity. Re-running bootstrap with the flag upgrades the same Helm release and reapplies the Application. Running without the flag does not install, update, or remove Cilium.

Because Cilium provides cluster networking, its Application deliberately has no cascading resource finalizer. Automated sync, pruning, and self-healing remain enabled, but deleting the Application cannot cascade into removal of the CNI.

## Configuration

Shared values live in `components/cilium/values/base.yaml`. Environment overrides live in `values/dev.yaml`, `values/staging.yaml`, and `values/prod.yaml`.

The base values add Cilium's supported `nonIdempotentAnnotations` setting so generated certificate resources do not leave the Application permanently out of sync when ArgoCD renders the chart.

Prepare the cluster for Cilium before enabling it. In particular, avoid a conflicting CNI and add any provider-specific Cilium values required by the target cluster. Commit the same configuration to the target Git revision before bootstrap so the ArgoCD handoff has no configuration drift.
