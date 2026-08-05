# Adding an Upstream Kueue Feature to the Operator

This guide describes the end-to-end workflow for enabling an upstream Kueue feature in the kueue-operator. Features fall into two categories based on their maturity in upstream Kueue, and the steps differ accordingly.

## 1. Determine Feature Maturity

Check the upstream Kueue docs and source for the feature's maturity level:

- **Beta or GA** — Enabled by default in upstream Kueue. The operator automatically deploys beta CRDs (skips only alpha CRDs with `v1alpha*` versions). No feature gate needed in the operator.
- **Alpha** — Requires an explicit feature gate in the Kueue Configuration. The operator must detect a trigger condition and set the gate in `buildFeatureGates()`.

| Maturity | CRD deployed? | Feature gate needed? | Operator CRD changes? |
|----------|---------------|----------------------|------------------------|
| Beta/GA | Yes (automatic) | No | Only if new user-facing config is needed |
| Alpha | Depends on CRD version | Yes | Yes (trigger condition + gate) |

## 2. Sync Upstream Manifests

Run `hack/sync_manifests.py` (or the equivalent make target) to pull the latest CRDs, ClusterRoles, and webhook configurations from the upstream Kueue submodule into `bindata/assets/kueue-operator/`.

Verify the new feature's resources appear:
- `bindata/assets/kueue-operator/crds/` — CRD YAML
- `bindata/assets/kueue-operator/clusterroles/` — Editor/viewer ClusterRoles
- `bindata/assets/kueue-operator/validatingwebhook.yaml` — Webhook entries
- `bindata/assets/kueue-operator/mutatingwebhook.yaml` — Mutating webhook entries (if any)

The operator's `manageCustomResources()` in `pkg/operator/target_config_reconciler.go` iterates all files in the CRD directory and deploys them automatically. Alpha CRDs (versions starting with `v1alpha`) are skipped.

## 3. Add Operator CRD Fields (if needed)

If the feature requires user-facing configuration in the operator's `Kueue` CR:

1. **Add fields** to `pkg/apis/kueueoperator/v1/types.go` under `KueueConfiguration`.
2. **Add validation** in `pkg/webhook/` if the field has constraints.
3. Run `make generate` to regenerate deepcopy, clients, and CRD manifests.

If the feature is purely upstream (no operator config surface), skip this step.

## 4. Enable Feature Gate (alpha features only)

For alpha features that require an explicit Kueue feature gate:

### 4a. Determine the trigger condition

Existing patterns for trigger conditions:

| Pattern | Example | Where |
|---------|---------|-------|
| Framework in integrations list | `SparkApplicationIntegration` | `configmap.go:buildFeatureGates()` |
| OpenShift FeatureGate CR check | `KueueDRAIntegrationExtendedResource` | `target_config_reconciler.go` sync loop |
| CRD field populated | `ShortWorkloadNames` | `configmap.go:buildFeatureGates()` |
| Cluster API discovery | `KueueDRAIntegration` | `target_config_reconciler.go` API check |

### 4b. Add detection logic

If the trigger depends on cluster state (e.g., an OpenShift FeatureGate):
- Add detection in `target_config_reconciler.go` within the `sync()` method.
- Store the result on the reconciler struct (e.g., `c.myFeatureEnabled`).
- Thread the boolean through to `BuildConfigMap()`.

### 4c. Set the feature gate

In `pkg/configmap/configmap.go`, add a block in `buildFeatureGates()`:

```go
if myFeatureEnabled {
    featureGates["MyFeatureGateName"] = true
}
```

### 4d. Add ConfigMap tests

In `pkg/configmap/configmap_test.go`, add test cases verifying the generated ConfigMap YAML includes the `featureGates:` section with the new gate enabled/disabled based on the trigger condition.

## 5. Add Webhook Handling (if needed)

If the feature introduces a new webhook annotation (e.g., `kueue.x-k8s.io/my-feature`):

1. Add the annotation constant and webhook name mapping in `pkg/webhook/pod_webhook.go`.
2. Determine if the webhook is cluster-scoped or namespace-scoped via `isClusterScopedFramework()`.

## 6. Write the Test Plan

Create `docs/plans/<FEATURE>_PLAN.md` using the template at `docs/plans/templates/feature-test-plan-template.md`. The plan should cover:

- **Upstream tests**: What upstream tests exist and what they cover.
- **Downstream tests**: Operator-specific scenarios (CRD deployment, webhook behavior, OpenShift-specific integration).

## 7. Add Test Utilities

Add wrapper types to `test/e2e/testutils/utils.go` following the existing pattern:

```go
type MyResourceWrapper struct {
    *kueuev1beta2.MyResource
}

func NewMyResource() *MyResourceWrapper {
    return &MyResourceWrapper{
        MyResource: &kueuev1beta2.MyResource{
            ObjectMeta: metav1.ObjectMeta{ GenerateName: "test-" },
            Spec: kueuev1beta2.MyResourceSpec{ /* defaults */ },
        },
    }
}

func (w *MyResourceWrapper) WithGenerateName() *MyResourceWrapper { /* ... */ }
func (w *MyResourceWrapper) CreateWithObject(ctx, client) (*kueuev1beta2.MyResource, func(), error) { /* ... */ }
```

Use `TestResourceBuilder` from `test/e2e/testutils/builders.go` for workload objects (Job, Pod, JobSet, etc.).

## 8. Write E2E Tests

Create `test/e2e/e2e_<feature>_test.go` with Ginkgo v2 labels:

```go
var _ = Describe("MyFeature", Label("operator", "myfeature"), Ordered, func() {
    // ...
})
```

Test categories:
- **CRD deployment**: Verify the CRD is registered by the operator.
- **CR lifecycle**: Create, read, update, delete the resource.
- **Feature behavior**: Validate the feature works end-to-end on OpenShift.
- **Webhook validation**: Verify invalid configurations are rejected.
- **Interaction with other features**: Test combinations (e.g., preemption + fair sharing).

## 9. Verify

```bash
make build                    # Compile
make test-unit                # Unit tests
make lint                     # Lint
make test-e2e                 # Downstream e2e
make e2e-upstream-test        # Upstream e2e suite
```

## 10. Checklist

- [ ] Upstream manifests synced (`hack/sync_manifests.py`)
- [ ] CRD appears in `bindata/assets/kueue-operator/crds/`
- [ ] ClusterRoles appear in `bindata/assets/kueue-operator/clusterroles/`
- [ ] Webhook entries present in validating/mutating webhook configs
- [ ] Feature gate added in `buildFeatureGates()` (alpha only)
- [ ] ConfigMap tests updated (alpha only)
- [ ] Operator CRD fields added (if user-facing config needed)
- [ ] `make generate` run after CRD type changes
- [ ] Test plan written in `docs/plans/`
- [ ] Test utilities added to `test/e2e/testutils/utils.go`
- [ ] E2E tests written in `test/e2e/e2e_<feature>_test.go`
- [ ] `make build && make test-unit && make lint` pass
- [ ] `make test-e2e` passes
