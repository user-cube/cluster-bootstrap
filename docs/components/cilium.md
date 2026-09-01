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
4. The CLI enables `components.cilium` on the root App of Apps Application.
5. The App of Apps creates an ArgoCD Application named `cilium`, pointing at `components/cilium/` in the configured repository and target revision.
6. ArgoCD renders the same chart version, release name, namespace, base values, and environment values used by bootstrap.

When the effective Cilium values enable any Prometheus `ServiceMonitor` (agent, operator, or Hubble), bootstrap first checks for the Prometheus Operator `ServiceMonitor` CRD. If it is absent, bootstrap installs the repository-pinned `prometheus-operator-crds` Helm component, waits for the API to become discoverable, and then installs Cilium. This preserves the usual Cilium-before-ArgoCD order while ensuring monitoring resources can be created.

This matching configuration lets ArgoCD take over the existing resources without a second release identity. Re-running bootstrap with the flag upgrades the same Helm release and reapplies the App of Apps configuration. Running without the flag does not install Cilium and leaves the default App of Apps manifest unchanged.

Because Cilium provides cluster networking, its Application deliberately has no cascading resource finalizer. Automated sync, pruning, and self-healing remain enabled, but deleting the Application cannot cascade into removal of the CNI.

## Configuration

Shared values live in `components/cilium/values/base.yaml`. Environment overrides live in `values/dev.yaml`, `values/staging.yaml`, and `values/prod.yaml`.

The optional child Application is declared as `components.cilium.enabled: false` in `apps/values.yaml`. The CLI overrides that value only on the root Application when `--enable-cilium` is passed.

The base values add Cilium's supported `nonIdempotentAnnotations` setting so generated certificate resources do not leave the Application permanently out of sync when ArgoCD renders the chart.

Prepare the cluster for Cilium before enabling it. In particular, avoid a conflicting CNI and add any provider-specific Cilium values required by the target cluster. Commit the same configuration to the target Git revision before bootstrap so the ArgoCD handoff has no configuration drift.
