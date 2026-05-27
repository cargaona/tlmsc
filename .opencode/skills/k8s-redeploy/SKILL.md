---
name: k8s-redeploy
description: Redeploy tlmsc bot to Kubernetes using latest image from GHCR
---

## K8s Redeploy

Use this workflow to deploy the latest version of the tlmsc bot to the Kubernetes cluster on kramer (192.168.88.253).

## Prerequisites

- `KUBECONFIG=~/.kube/config2` (kramer cluster)
- `gh` CLI authenticated with GitHub
- `kubectl` available
- Manifests live at `/home/char/projects/personal/code/kube-server/manifests/namespaces/tlmsc/`

All kubectl commands below must be prefixed with:

```bash
export KUBECONFIG=~/.kube/config2
```

## Image-only redeploy (code changes only)

Pushing or merging to `develop` triggers CI which builds and pushes `ghcr.io/cargaona/tlmsc:latest`. The deployment has `imagePullPolicy: Always`, so a rollout restart pulls the new image.

### 1. Wait for CI to finish

```bash
gh run list -w "Build and Publish Docker Image" -L 1
```

Confirm status is `completed` and conclusion is `success`. If still running, wait:

```bash
gh run watch <run-id>
```

### 2. Restart the deployment

```bash
kubectl rollout restart deployment/tlmsc -n tlmsc
kubectl rollout status deployment/tlmsc -n tlmsc --timeout=120s
```

## Manifest changes (env vars, configmap, storage, etc.)

When K8s manifests change (deployment.yaml, configmap.yaml, storage.yaml, secrets.yaml), apply them from the kube-server repo:

```bash
kubectl apply -f /home/char/projects/personal/code/kube-server/manifests/namespaces/tlmsc/
```

If only a specific file changed:

```bash
kubectl apply -f /home/char/projects/personal/code/kube-server/manifests/namespaces/tlmsc/<file>.yaml
```

A rollout restart may still be needed if the deployment spec didn't change but a ConfigMap or Secret did.

## Verification

```bash
# Check pod is running
kubectl get pods -n tlmsc

# Check logs for startup success (look for "Bot @... started successfully")
kubectl logs -n tlmsc deploy/tlmsc --tail=20

# Verify beets config is loaded
kubectl exec -n tlmsc deploy/tlmsc -- beet config

# Verify library is accessible
kubectl exec -n tlmsc deploy/tlmsc -- beet stats
```

## Rollback

If something is wrong, roll back to the previous revision:

```bash
kubectl rollout undo deployment/tlmsc -n tlmsc
kubectl rollout status deployment/tlmsc -n tlmsc --timeout=120s
```
