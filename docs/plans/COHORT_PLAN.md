# Test Plan for Cohorts

**Plan Status:** Draft
**Feature:** Cohort Resource Sharing and Hierarchical Cohort Trees
**Testing Epic:** TBD

## Overview

- [References](#references) — KEPs, JIRA tickets, docs, and known bugs
- [Introduction](#introduction) — What the feature is and what changed
- [Test Strategy](#test-strategy) — Upstream vs downstream approach
- [Test Scope](#test-scope) — Upstream and downstream scenarios
- [Out of Scope](#out-of-scope) — What we're not testing and why
- [Target Environments](#target-environments) — OCP versions, architectures, FIPS, disconnected, Hypershift
- [Test Deliverables](#test-deliverables) — PRs and test reports we produce
- [Test Tasks](#test-tasks) — Work breakdown
- [Pass/Fail Criteria](#passfail-criteria) — Exit criteria
- [Risks](#risks) — Blockers and unknowns

## References

| Type | Link |
|------|------|
| Testing Epic | TBD |
| Upstream docs | [Kueue Cohort Concepts](https://kueue.sigs.k8s.io/docs/concepts/cohort/) |
| Cohort CRD (operator) | [`bindata/assets/kueue-operator/crds/crd-cohorts.kueue.x-k8s.io-v1beta2.yaml`](../../bindata/assets/kueue-operator/crds/crd-cohorts.kueue.x-k8s.io-v1beta2.yaml) |
| Cohort API types | [`vendor/sigs.k8s.io/kueue/apis/kueue/v1beta2/cohort_types.go`](../../vendor/sigs.k8s.io/kueue/apis/kueue/v1beta2/cohort_types.go) |
| Existing downstream test (preemption within cohort) | [`test/e2e/e2e_preemption_test.go`](../../test/e2e/e2e_preemption_test.go) |
| Test utilities (WithCohort, WithBorrowingLimit) | [`test/e2e/testutils/utils.go`](../../test/e2e/testutils/utils.go) |
| Validating webhook | [`bindata/assets/kueue-operator/validatingwebhook.yaml`](../../bindata/assets/kueue-operator/validatingwebhook.yaml) (webhook `vcohort.kb.io`) |
| Bugs | _none known_ |

## Introduction

Cohorts are a cluster-scoped Kueue resource (`kueue.x-k8s.io/v1beta2`) that enable resource sharing between ClusterQueues. ClusterQueues that belong to the same Cohort (or the same Cohort tree) can borrow and lend resources from one another, subject to configurable limits.

The Cohort API (v1beta2) supports:

- **Implicit grouping**: ClusterQueues join a Cohort by setting `spec.cohortName` to the same value. Even without an explicit Cohort CR, ClusterQueues with matching `cohortName` share resources.
- **Explicit Cohort CRs**: A `Cohort` custom resource can be created to define additional shared resource pools via `spec.resourceGroups`. The `nominalQuota` at the Cohort level represents extra resources on top of what member ClusterQueues define. ClusterQueues must define a nominal quota (even if 0) for a resource in order to borrow it from the Cohort.
- **Hierarchical Cohort trees**: Cohorts can reference a parent via `spec.parentName`, forming tree structures. Borrowing and lending limits can be set at each level. Cycle detection disables all affected members.
- **Fair Sharing**: Cohorts support `spec.fairSharing.weight` for proportional resource distribution among sibling Cohorts in a tree.

The Cohort feature is a beta API — there is no feature gate required. The operator deploys the Cohort CRD, ClusterRoles (editor/viewer), and validating webhook unconditionally as part of standard reconciliation. No operator CRD changes are needed to enable Cohorts.

**Current state in the operator:**
- The Cohort CRD is deployed automatically.
- The existing preemption e2e test (`e2e_preemption_test.go`) validates implicit cohort resource borrowing and reclaim-within-cohort preemption.
- No tests currently create explicit `Cohort` CR objects or exercise hierarchical Cohort trees.

## Test Strategy

- **Upstream:** Cohort functionality is covered by unit and integration tests in the upstream `kubernetes-sigs/kueue` repository. The operator runs the upstream e2e suite via `make e2e-upstream-test`, which validates Cohort behavior on the current OpenShift cluster with the upstream test harness.

- **Downstream:** Operator-specific e2e tests are needed for scenarios that depend on the operator's deployment model (CRD installation, webhook configuration, OpenShift-specific namespace management) and for validating explicit Cohort CR lifecycle on OpenShift. Existing test utilities (`WithCohort`, `WithBorrowingLimit`, `WithReclaimWithinCohort`) should be extended with a `CohortWrapper` for creating explicit Cohort CRs. The existing preemption test already covers basic implicit cohort sharing and can be leveraged as a pattern.

## Test Scope

### Upstream Tests

Tests run via `make e2e-upstream-test` from the `upstream/kueue` submodule.

| ID | Scenario | What It Validates |
|----|----------|-------------------|
| T1 | ClusterQueues with same cohortName share resources | Implicit cohort grouping enables resource borrowing |
| T2 | BorrowingLimit restricts how much a CQ borrows | Limits are enforced on resource borrowing within a cohort |
| T3 | LendingLimit restricts how much a CQ lends | Limits are enforced on resource lending within a cohort |
| T4 | ReclaimWithinCohort preempts borrowing workloads | Workloads borrowing from cohort are evicted when the lending CQ needs resources |
| T5 | Cohort CR with resourceGroups adds shared pool | Explicit Cohort quota supplements CQ-defined quota |
| T6 | Hierarchical Cohort tree resource sharing | Parent/child Cohorts share resources across levels |
| T7 | FairSharing weight distributes resources proportionally | Sibling Cohorts with different weights trend toward proportional usage |
| T8 | Cohort cycle detection disables members | Circular parent references disable all affected CQs and Cohorts |

### Downstream Tests

New tests in `test/e2e/e2e_cohort_test.go`, labeled `Label("operator", "cohort")`.

| ID | Scenario | Sub-task | Why downstream-specific |
|----|----------|----------|------------------------|
| D1 | Cohort CRD is registered by the operator | TBD | Validates operator deploys the CRD correctly; already partially covered in `e2e_operator_test.go` CRD check list |
| D2 | Create explicit Cohort CR with resourceGroups | TBD | Validates Cohort CR lifecycle on OpenShift with operator-managed webhooks |
| D3 | ClusterQueue borrows from Cohort shared pool | TBD | Validates CQ with `nominalQuota: 0` can borrow from Cohort-level resourceGroups on OpenShift |
| D4 | Hierarchical Cohort tree: child borrows from parent | TBD | Validates hierarchical resource sharing with operator-deployed CRD and webhook conversion (v1beta1 <-> v1beta2) |
| D5 | BorrowingLimit and LendingLimit on Cohort resourceGroups | TBD | Validates limits are enforced at the Cohort level with operator webhook |
| D6 | Cohort validating webhook rejects invalid config | TBD | Validates the operator-deployed webhook (`vcohort.kb.io`) rejects Cohorts with borrowing/lending limits but no parent |
| D7 | Cohort with FairSharing and preemption | TBD | Validates end-to-end fair sharing within a cohort tree on OpenShift, extending existing preemption test patterns |
| D8 | Cohort deletion cascading behavior | TBD | Validates that deleting a Cohort CR does not orphan or break member ClusterQueues |

## Out of Scope

- **Multi-cluster Cohort scenarios** — MultiKueue is a separate feature with its own test plan
- **Performance / scale testing** — Cohort behavior with hundreds of ClusterQueues is outside e2e scope
- **Cohort metrics** — Prometheus metric validation for Cohort-related metrics (fairSharing weightedShare) is deferred

### Scenarios considered and excluded

| Scenario | Reason |
|----------|--------|
| Cohort with > 16 resource groups | CRD validation enforces the limit; webhook coverage is sufficient |
| v1beta1 Cohort API direct testing | v1beta1 is deprecated; v1beta2 is the storage version. Conversion webhook is tested implicitly in D4 |
| Cohort interaction with TopologyAwareScheduling | TAS has its own test plan and feature gate; Cohort + TAS intersection is not yet defined upstream |

## Target Environments

- x86_64
- ARM (aarch64)
- FIPS
- Disconnected
- OCP versions: 4.19+ (current development target)
- Hypershift — HCP

## Test Deliverables

- Downstream PR with `test/e2e/e2e_cohort_test.go` in `openshift/kueue-operator` (Prow CI)
- `CohortWrapper` test utility additions in `test/e2e/testutils/utils.go` for creating explicit Cohort CRs
- Test report with information for Docs team

## Test Tasks

1. Add `CohortWrapper` to `test/e2e/testutils/utils.go` with fluent builder methods: `WithGenerateName()`, `WithParentName()`, `WithResourceGroups()`, `WithFairSharingWeight()`, `CreateWithObject()`
2. Implement D1–D3: Cohort CRD verification, explicit Cohort CR creation, and borrowing from Cohort shared pool
3. Implement D4–D5: Hierarchical Cohort tree and borrowing/lending limits
4. Implement D6: Webhook validation rejection tests
5. Implement D7–D8: Fair sharing with preemption and Cohort deletion behavior
6. Verify all tests pass in CI (`make test-e2e`)

## Pass/Fail Criteria

- No critical or major defects remain open
- All downstream e2e tests (D1–D8) pass consistently in CI
- Upstream e2e suite (`make e2e-upstream-test`) continues to pass without regressions
- Cohort CRD, ClusterRoles, and webhook are deployed correctly by the operator

## Risks

| Risk | Impact |
|------|--------|
| Cohort behavior depends entirely on upstream Kueue controller — operator only deploys CRD/webhook | Bugs in upstream Cohort controller would cause downstream test failures; mitigation is to file upstream issues |
| Hierarchical Cohort trees are a newer upstream feature | May have edge cases not yet covered by upstream tests; downstream tests may discover new issues |
| CohortWrapper does not exist in test utilities yet | Must be implemented before downstream Cohort tests can be written; follow existing patterns from `ClusterQueueWrapper` and `ResourceFlavorWrapper` |
| Cohort webhook conversion (v1beta1 <-> v1beta2) is auto-generated | If upstream changes conversion logic, operator must sync manifests; test D4 validates this path |
