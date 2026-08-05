This is the kueue operator. It enables features in the upstream kueue project https://github.com/kubernetes-sigs/kueue . The source is also in upstream/kueue/src. the docs for the upstream feature are
https://kueue.sigs.k8s.io/docs/concepts/cohort/. Write me a plan file so that we can reproduce enabling the upstream feature in the operator. The operator automatically enables beta APIs. using this template
https://raw.githubusercontent.com/anahas-redhat/kueue-operator/3d7e8b7ee2d78d69608c147caad79d088c5e6dec/docs/plans/templates/feature-test-plan-template.md write me a plan file in docs/plans/COHORT_PLAN.md

/new
read the docs/plans/COHORT_PLAN.md

implement the CohortWrapper

make sure to use the context passed into createwithoject in the cleanup function

what would you implement next

implement d2 and d3

implement d4 and d5
