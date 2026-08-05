---
name: kind-e2e
description: Build docker images from related_images.rphillips.json, deploy the kueue-operator to a kind cluster via OLM bundle, and run downstream and upstream e2e tests. Use when asked to "test on kind", "run e2e locally", "build and test", "deploy to kind", or similar local development testing requests.
---

# Kind E2E: Build, Deploy, and Test kueue-operator Locally

Builds all docker images defined in `related_images.rphillips.json`, pushes them to
`quay.io/ryan.phillips/`, deploys the operator to a kind cluster via OLM bundle,
and runs downstream and upstream e2e tests.

## Step 0: Prerequisites

Ensure these tools are available (use `nix-shell -p` if needed):

```bash
which docker kind kubectl go operator-sdk
```

Ensure the git submodule is initialized:

```bash
git submodule update --init
```

Ensure Docker daemon is running:

```bash
docker info >/dev/null 2>&1 || echo "Docker is not running"
```

## Step 1: Build Images

Read `related_images.rphillips.json` to get the image tags. Build all images using
the existing Dockerfiles:

### Operator

```bash
docker build -f Dockerfile.ci -t $(jq -r '.[] | select(.name == "operator") | .image' related_images.rphillips.json) .
```

### Operand

```bash
docker build -f Dockerfile.ci.kueue -t $(jq -r '.[] | select(.name == "operand") | .image' related_images.rphillips.json) .
```

### Must-gather

```bash
docker build -f Dockerfile.ci.must-gather -t $(jq -r '.[] | select(.name == "must-gather") | .image' related_images.rphillips.json) .
```

### Bundle

First generate the bundle manifests, then build with the custom related images file:

```bash
make bundle-generate
docker build -f bundle.Dockerfile \
  --build-arg RELATED_IMAGE_FILE=related_images.rphillips.json \
  -t $(jq -r '.[] | select(.name == "bundle") | .image' related_images.rphillips.json) .
```

## Step 2: Push Images

Push all images to quay.io:

```bash
for name in operator operand must-gather bundle; do
  img=$(jq -r --arg n "$name" '.[] | select(.name == $n) | .image' related_images.rphillips.json)
  if [ -n "$img" ]; then
    docker push "$img"
  fi
done
```

## Step 3: Create Kind Cluster

Check if a cluster already exists, create if not:

```bash
if ! kind get clusters 2>/dev/null | grep -q '^kueue-operator$'; then
  kind create cluster --name kueue-operator --config hack/kind-cluster.yaml
fi
```

Set kubectl context:

```bash
kubectl cluster-info --context kind-kueue-operator
```

Verify nodes (should see 1 control-plane + 2 workers):

```bash
kubectl get nodes --show-labels
```

## Step 4: Install OLM

Install Operator Lifecycle Manager on the kind cluster:

```bash
operator-sdk olm install
```

Wait for OLM to be ready:

```bash
operator-sdk olm status
```

## Step 5: Deploy cert-manager

Install upstream cert-manager (required by the operator for webhook certificates):

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.17.0/cert-manager.yaml
```

Wait for cert-manager to be ready:

```bash
kubectl wait --for=condition=Available deployment/cert-manager -n cert-manager --timeout=300s
kubectl wait --for=condition=Available deployment/cert-manager-webhook -n cert-manager --timeout=300s
kubectl wait --for=condition=Available deployment/cert-manager-cainjector -n cert-manager --timeout=300s
```

## Step 6: Install JobSet and LeaderWorkerSet

Install the upstream JobSet controller (v0.12.0, matching go.mod):

```bash
kubectl apply --server-side -f https://github.com/kubernetes-sigs/jobset/releases/download/v0.12.0/manifests.yaml
kubectl wait --for=condition=Available deployment/jobset-controller-manager -n jobset-system --timeout=300s
```

Install the upstream LeaderWorkerSet controller (v0.8.0, matching go.mod):

```bash
kubectl apply --server-side -f https://github.com/kubernetes-sigs/lws/releases/download/v0.8.0/manifests.yaml
kubectl wait --for=condition=Available deployment/lws-controller-manager -n lws-system --timeout=300s
```

## Step 7: Deploy Operator via Bundle

Create the operator namespace:

```bash
kubectl apply -f deploy/01_namespace.yaml
```

Deploy the operator using the OLM bundle:

```bash
BUNDLE_IMAGE=$(jq -r '.[] | select(.name == "bundle") | .image' related_images.rphillips.json)
operator-sdk run bundle "$BUNDLE_IMAGE" \
  --namespace openshift-kueue-operator \
  --timeout 5m
```

Verify the operator is running:

```bash
kubectl get pods -n openshift-kueue-operator
kubectl wait --for=condition=Available deployment/openshift-kueue-operator \
  -n openshift-kueue-operator --timeout=300s
```

## Step 8: Create Kueue CR

Apply the default Kueue operand configuration:

```bash
kubectl apply -f test/e2e/bindata/assets/08_kueue_default.yaml
```

Wait for the kueue controller manager to be ready:

```bash
timeout 300s bash -c 'until kubectl get deployment kueue-controller-manager -n openshift-kueue-operator -o jsonpath="{.status.conditions[?(@.type==\"Available\")].status}" 2>/dev/null | grep -q "True"; do sleep 10; echo "Waiting for kueue-controller-manager..."; done'
echo "kueue-controller-manager is ready"
```

Verify all Kueue CRDs are installed:

```bash
kubectl get crds | grep kueue
```

## Step 9: Run Downstream E2E Tests

Run the operator's own e2e test suite. Start with non-disruptive tests:

```bash
make test-e2e
```

For CI-style runs (excludes disruptive and flaky tests):

```bash
make e2e-ci-test
```

To run specific test suites by label:

```bash
# Operator tests only
./bin/ginkgo --label-filter="operator" -v ./test/e2e/...

# DRA tests (requires DRA-capable cluster)
./bin/ginkgo --label-filter="dra" -v ./test/e2e/...
```

## Step 10: Run Upstream E2E Tests

Label the worker nodes for upstream tests (should already have labels from kind config,
but verify):

```bash
kubectl get nodes -l instance-type=on-demand
kubectl get nodes -l instance-type=spot
```

Run the upstream Kueue e2e tests:

```bash
make e2e-upstream-test
```

To run specific upstream test folders:

```bash
E2E_TARGET_FOLDERS="singlecluster" make e2e-upstream-test
```

## Step 11: Teardown

Delete the kind cluster when done:

```bash
kind delete cluster --name kueue-operator
```

## Rebuilding After Code Changes

To iterate after making code changes:

1. Rebuild the changed image(s) (Step 1)
2. Push the updated image(s) (Step 2)
3. If operator code changed, restart the operator:
   ```bash
   kubectl rollout restart deployment/openshift-kueue-operator -n openshift-kueue-operator
   ```
4. If operand code changed, delete and recreate the Kueue CR to trigger re-deployment:
   ```bash
   kubectl delete kueue cluster -n openshift-kueue-operator
   kubectl apply -f test/e2e/bindata/assets/08_kueue_default.yaml
   ```
5. Re-run tests (Steps 8-9)

## Troubleshooting

- **Image pull errors in kind**: Ensure images were pushed to quay.io and are publicly accessible. Check `docker push` output for auth errors.
- **OLM install fails**: Check `operator-sdk` version compatibility. Try `operator-sdk olm uninstall && operator-sdk olm install`.
- **cert-manager not ready**: Check pod logs: `kubectl logs -n cert-manager -l app=cert-manager`.
- **Operator CrashLoopBackOff**: Check logs: `kubectl logs -n openshift-kueue-operator -l name=openshift-kueue-operator`.
- **kueue-controller-manager not starting**: Check operator logs and events: `kubectl describe kueue cluster -n openshift-kueue-operator`.
