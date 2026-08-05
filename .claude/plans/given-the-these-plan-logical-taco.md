# Cohort E2E Test Implementation Plan

## Context

The `COHORT_PLAN.md` defines 8 downstream e2e test scenarios (D1-D8) for Kueue Cohort resource sharing. This worktree starts from `main`, which has no Cohort tests or CohortWrapper. The `add_cohort_plan` branch has a prior implementation, but we're reimplementing cleanly from main with improvements identified during review.

**Key gaps in the prior implementation to address:**
- CohortWrapper cleanup function uses the captured `ctx` from `CreateWithObject` instead of `context.TODO()` like every other wrapper (inconsistent with ClusterQueueWrapper, ResourceFlavorWrapper, TopologyWrapper patterns)
- D7 doesn't actually test `FairSharing.Weight` on a Cohort CR despite the plan title saying "FairSharing weight" — it only tests implicit cohort ReclaimWithinCohort preemption (the `WithFairSharingWeight` builder method goes unused)
- D5 only tests `BorrowingLimit`, not `LendingLimit` on resourceGroups as the plan title implies
- `checkWorkloadCondition` is defined in `e2e_preemption_test.go` and shared across test files — the cohort tests depend on it being in the same package
- The preemption test at line 134 has an inline `FlavorsUsage` check that duplicates what would be `CheckBorrowedCPU` — should use the shared utility

## Files to modify

1. **`test/e2e/testutils/utils.go`** — Add `CohortWrapper` struct and methods, add `CheckBorrowedCPU` utility
2. **`test/e2e/e2e_cohort_test.go`** — New file with D2-D8 tests
3. **`test/e2e/e2e_preemption_test.go`** — Refactor inline `FlavorsUsage` check at line 134 to use `CheckBorrowedCPU`

D1 is already covered by `e2e_operator_test.go:1783` (`"cohorts.kueue.x-k8s.io"` in `requiredCRDs`).

## Implementation Steps

### Step 1: Add CohortWrapper to `test/e2e/testutils/utils.go`

Add after the `TopologyWrapper` section (after line 491):

- `CohortWrapper` struct wrapping `*kueuev1beta2.Cohort`
- `NewCohort()` — default name `"test-cohort"`, empty `CohortSpec`
- `WithGenerateName()` — prefix `"cohort-"`
- `WithParentName(parent string)` — sets `spec.parentName`
- `WithResourceGroups(rgs []kueuev1beta2.ResourceGroup)` — sets `spec.resourceGroups`
- `WithFairSharingWeight(weight string)` — sets `spec.fairSharing.weight`
- `Create(ctx, client)` — returns `(cleanup, error)`
- `CreateWithObject(ctx, client)` — returns `(*Cohort, cleanup, error)`

Cleanup function pattern: use `context.TODO()` (matching ClusterQueueWrapper/ResourceFlavorWrapper/TopologyWrapper), `removeFinalizersWithPatch`, delete, `Eventually` wait for deletion.

### Step 2: Add `CheckBorrowedCPU` to `test/e2e/testutils/utils.go`

Add after `IsPodScheduled`:

```go
func CheckBorrowedCPU(ctx context.Context, kueueClient *upstreamkueueclient.Clientset, cqName, minBorrowed, description string)
```

Polls `ClusterQueue.Status.FlavorsUsage`, finds CPU resource, asserts `Borrowed >= minBorrowed`.

### Step 3: Create `test/e2e/e2e_cohort_test.go`

Labels: `Label("operator", "cohort")`, `Ordered`

Uses existing helpers:
- `testutils.NewResourceFlavor()`, `NewClusterQueue()`, `NewLocalQueue()`, `NewCohort()`, `CreateNamespace()`, `NewTestResourceBuilder()`, `CleanUpJob()`, `CheckBorrowedCPU()`
- `checkWorkloadCondition()` from `e2e_preemption_test.go` (same package)

**D2**: Create explicit Cohort CR with resourceGroups, verify it exists and spec is correct.

**D3**: Create Cohort with shared pool, CQ with nominalQuota 0 joined to cohort, submit Job, verify admitted and `CheckBorrowedCPU`.

**D4**: Create parent Cohort with resources, child Cohort with `WithParentName`, CQ in child with nominalQuota 0, submit Job, verify admitted and borrowed from parent.

**D5**: Create parent Cohort (4 CPU), child Cohort with `BorrowingLimit` of 500m CPU, CQ in child with nominalQuota 0. First job (250m) admitted. Second job (500m) stays pending (`Consistently`).

**D6**: Attempt to create Cohort with `BorrowingLimit` but no `parentName` — expect webhook error. Same for `LendingLimit`.

**D7**: Two CQs in implicit cohort. CQ-A has `BorrowingLimit` and `ReclaimWithinCohort: Any`. Job A borrows, Job B reclaims, verify Job A evicted.

**D8**: Create Cohort + CQ, verify CQ Active, delete Cohort, verify CQ remains Active and can admit workloads on its own quota.

### Step 4: Refactor `e2e_preemption_test.go` inline borrowed check

Replace the inline `Eventually` block at lines 134-151 with:
```go
testutils.CheckBorrowedCPU(ctx, clients.UpstreamKueueClient, clusterQueueA.Name, "250m", "clusterQueueA should have borrowed 250m CPU")
```

### Step 5: Verify

```bash
make build
make test-unit
make lint
```

On a cluster:
```bash
make test-e2e  # runs all non-disruptive e2e tests including cohort
```

Or targeted:
```bash
go test ./test/e2e/ -v -ginkgo.label-filter="cohort" -count=1
```
