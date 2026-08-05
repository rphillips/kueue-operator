---
name: add-upstream-feature
description: Research an upstream Kueue feature, assess operator integration needs, generate a test plan, and scaffold implementation
---

export const meta = {
  name: 'add-upstream-feature',
  description: 'Research an upstream Kueue feature, assess operator integration, generate test plan and scaffold',
  phases: [
    { title: 'Research', detail: 'Research upstream feature docs, feature gates, and existing operator support' },
    { title: 'Assess', detail: 'Determine what operator changes are needed (CRD, feature gate, webhooks, tests)' },
    { title: 'Plan', detail: 'Generate test plan document and implementation checklist' },
  ],
}

const featureName = args?.feature || 'unknown-feature'
const featureDocsUrl = args?.docsUrl || null
const jiraEpic = args?.jiraEpic || 'TBD'

// Phase 1: Research
phase('Research')

const [upstreamResearch, operatorState] = await parallel([
  () => agent(`Research the upstream Kueue feature "${featureName}" for the kueue-operator project.

Search for:
1. Feature gate name (if any) in vendor/sigs.k8s.io/kueue/ - look for feature gate registrations containing "${featureName}"
2. CRD types and API fields in vendor/sigs.k8s.io/kueue/apis/kueue/v1beta2/ related to "${featureName}"
3. Whether the feature is alpha, beta, or GA in upstream Kueue
4. Any configuration fields in vendor/sigs.k8s.io/kueue/apis/config/v1beta2/configuration_types.go related to this feature
${featureDocsUrl ? '5. Fetch the upstream docs at ' + featureDocsUrl + ' for feature details' : ''}

Return a structured summary:
- Feature name
- Maturity level (alpha/beta/GA)
- Feature gate name (if any, or "none")
- CRD kind(s) involved
- Key API fields and their types
- Configuration fields (if any)
- Brief description of what the feature does`, {
    label: 'research:upstream',
    phase: 'Research',
    schema: {
      type: 'object',
      properties: {
        featureName: { type: 'string' },
        maturity: { type: 'string', enum: ['alpha', 'beta', 'ga', 'unknown'] },
        featureGateName: { type: 'string', description: 'Name of feature gate or "none"' },
        crdKinds: { type: 'array', items: { type: 'string' } },
        apiFields: { type: 'array', items: {
          type: 'object',
          properties: {
            field: { type: 'string' },
            type: { type: 'string' },
            description: { type: 'string' },
          },
          required: ['field', 'type', 'description'],
        }},
        configFields: { type: 'array', items: { type: 'string' } },
        description: { type: 'string' },
      },
      required: ['featureName', 'maturity', 'featureGateName', 'crdKinds', 'apiFields', 'description'],
    },
  }),

  () => agent(`Assess the current state of support for the "${featureName}" feature in the kueue-operator.

Check:
1. bindata/assets/kueue-operator/crds/ - is there a CRD for this feature already deployed?
2. bindata/assets/kueue-operator/clusterroles/ - are there editor/viewer ClusterRoles?
3. bindata/assets/kueue-operator/validatingwebhook.yaml - is there a validating webhook?
4. bindata/assets/kueue-operator/mutatingwebhook.yaml - is there a mutating webhook?
5. pkg/configmap/configmap.go buildFeatureGates() - is the feature gate already handled?
6. pkg/operator/target_config_reconciler.go - any detection or handling logic?
7. pkg/apis/kueueoperator/v1/types.go - any operator CRD fields for this feature?
8. test/e2e/ - any existing e2e tests?
9. test/e2e/testutils/utils.go - any test wrapper/utility support?
10. pkg/webhook/pod_webhook.go - any webhook annotation handling?

Return a structured assessment.`, {
    label: 'research:operator',
    phase: 'Research',
    schema: {
      type: 'object',
      properties: {
        crdDeployed: { type: 'boolean' },
        crdPath: { type: 'string' },
        clusterRolesExist: { type: 'boolean' },
        validatingWebhookExists: { type: 'boolean' },
        mutatingWebhookExists: { type: 'boolean' },
        featureGateHandled: { type: 'boolean' },
        featureGateDetails: { type: 'string' },
        operatorCRDFields: { type: 'boolean' },
        operatorCRDFieldDetails: { type: 'string' },
        existingTests: { type: 'array', items: { type: 'string' } },
        testUtilitiesExist: { type: 'boolean' },
        testUtilityDetails: { type: 'string' },
        webhookAnnotationHandled: { type: 'boolean' },
      },
      required: ['crdDeployed', 'featureGateHandled', 'existingTests', 'testUtilitiesExist'],
    },
  }),
])

log(`Research complete for "${featureName}": maturity=${upstreamResearch?.maturity}, gate=${upstreamResearch?.featureGateName}, CRD deployed=${operatorState?.crdDeployed}`)

// Phase 2: Assess
phase('Assess')

const assessment = await agent(`You are assessing what work is needed to fully enable the "${featureName}" feature in the kueue-operator.

Upstream research findings:
${JSON.stringify(upstreamResearch, null, 2)}

Current operator state:
${JSON.stringify(operatorState, null, 2)}

Based on the operator's patterns (documented in docs/plans/templates/adding-a-feature.md), determine what changes are needed.

The operator's rules:
- Beta/GA CRDs are deployed automatically (alpha CRDs with v1alpha* versions are skipped)
- Alpha feature gates must be explicitly enabled in pkg/configmap/configmap.go buildFeatureGates()
- Feature gate triggers can be: framework in integrations list, OpenShift FeatureGate CR check, CRD field populated, or cluster API discovery
- Test utilities go in test/e2e/testutils/utils.go as wrapper types with fluent builders
- E2E tests go in test/e2e/e2e_<feature>_test.go with Label("operator", "<feature>")

Produce an implementation checklist.`, {
  label: 'assess:changes-needed',
  phase: 'Assess',
  schema: {
    type: 'object',
    properties: {
      needsManifestSync: { type: 'boolean', description: 'Need to run hack/sync_manifests.py' },
      needsFeatureGate: { type: 'boolean', description: 'Need to add feature gate in buildFeatureGates()' },
      featureGateTrigger: { type: 'string', description: 'How the gate should be triggered' },
      needsOperatorCRDFields: { type: 'boolean', description: 'Need new fields in operator Kueue CRD' },
      proposedCRDFields: { type: 'array', items: { type: 'string' } },
      needsWebhookHandling: { type: 'boolean', description: 'Need webhook annotation in pod_webhook.go' },
      needsTestUtilities: { type: 'boolean', description: 'Need new wrapper types in testutils' },
      proposedWrapperTypes: { type: 'array', items: { type: 'string' } },
      needsE2ETests: { type: 'boolean' },
      proposedTestScenarios: {
        type: 'array',
        items: {
          type: 'object',
          properties: {
            id: { type: 'string' },
            scenario: { type: 'string' },
            whyDownstream: { type: 'string' },
          },
          required: ['id', 'scenario'],
        },
      },
      summary: { type: 'string', description: 'One-paragraph summary of all needed changes' },
    },
    required: ['needsManifestSync', 'needsFeatureGate', 'needsOperatorCRDFields', 'needsTestUtilities', 'needsE2ETests', 'summary'],
  },
})

log(`Assessment complete: gate=${assessment?.needsFeatureGate}, CRD fields=${assessment?.needsOperatorCRDFields}, tests=${assessment?.needsE2ETests}`)

// Phase 3: Plan
phase('Plan')

const testPlan = await agent(`Generate a test plan document for the "${featureName}" feature in the kueue-operator.

Use EXACTLY this template structure (from docs/plans/templates/feature-test-plan-template.md):

# Test Plan for [Feature Name]

**Plan Status:** Draft
**Feature:** [Feature title]
**Testing Epic:** [JIRA link]

## Overview
(table of contents)

## References
(table of links)

## Introduction
(2-3 paragraphs)

## Test Strategy
(upstream vs downstream approach)

## Test Scope
### Upstream Tests
(table: ID, Scenario, What It Validates)
### Downstream Tests
(table: ID, Scenario, Sub-task, Why downstream-specific)

## Out of Scope
(list with reasons)
### Scenarios considered and excluded
(table)

## Target Environments
(list)

## Test Deliverables
(list)

## Test Tasks
(numbered list with JIRA links)

## Pass/Fail Criteria
(list)

## Risks
(table)

Fill it in with these details:

Feature research:
${JSON.stringify(upstreamResearch, null, 2)}

Operator state:
${JSON.stringify(operatorState, null, 2)}

Implementation assessment:
${JSON.stringify(assessment, null, 2)}

JIRA Epic: ${jiraEpic}

Write the COMPLETE markdown document. Do not use placeholders except for JIRA ticket numbers (use TBD).
Include at least 4 upstream test scenarios and at least 6 downstream test scenarios.
Target environments: x86_64, ARM (aarch64), FIPS, Disconnected, OCP 4.19+, Hypershift — HCP.`, {
  label: 'plan:test-plan',
  phase: 'Plan',
})

return {
  upstream: upstreamResearch,
  operatorState: operatorState,
  assessment: assessment,
  testPlan: testPlan,
}
