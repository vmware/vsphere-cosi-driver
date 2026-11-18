# Object store operator

One operator runs for each object store, in the store's namespace. It creates and deletes the
store's buckets for `ObjectStoreBucket` resources and reports the result in their status.

## Install

The files are written for any namespace. Install them through kustomize with the store's
namespace set, which also fills in the RoleBinding's subject:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: <the store's namespace>
resources:
  - <path to>/manifests/supervisor/operator
```

Installing the files without kustomize leaves the RoleBinding without a namespace, so it grants
nothing. The `ObjectStoreBucket` CRD in `manifests/supervisor/crd` must be installed too. Neither
folder is part of `manifests/supervisor/kustomization.yaml` yet.

The image tag `<object_store_operator_ver>` is a placeholder for the release process to replace.

## What the namespace must hold

- Exactly one `ObjectStore`. More than one is an error that is retried.
- Both Secrets below. They are mounted as optional, so the pod starts without them.

| Secret                      | Keys             | Who writes it    | Used by                                           |
|-----------------------------|------------------|------------------|---------------------------------------------------|
| `vboss-admin-secret`        | `akey`, `secret` | The object store | The bootstrap, once, to create the operator's key |
| `cosi-operator-credentials` | `akey`, `secret` | The bootstrap    | The reconciler, on every reconcile                |

The Deployment mounts each Secret read-only with only the two key files.

## Configuration

| Variable             | Default                      | Meaning                                                      |
|----------------------|------------------------------|--------------------------------------------------------------|
| POD_NAMESPACE        | none, required               | The namespace the operator serves. Set from the downward API |
| CREDENTIALS_DIR      | `/etc/cosi/credentials`      | Where the operator's own key is mounted                      |
| ROOT_CREDENTIALS_DIR | `/etc/cosi/root-credentials` | Where the store's root credentials are mounted               |
| LOG_LEVEL            | `info`                       | `debug`, `info`, `warn` or `error`                           |
| METRICS_ADDR         | `:8080`                      | Metrics listen address                                       |
| PROBE_ADDR           | `:8081`                      | Health probe listen address (`/healthz`, `/readyz`)          |

## Starting up

The operator needs its own key to reach the store. With the root credentials it creates a user
named `cosi-operator`, attaches a policy and creates a key, and keeps the key in
`cosi-operator-credentials`, owned by the `ObjectStore`. It waits until the store is ready and the
root credentials are mounted, and a Secret that already holds a key means nothing is created.

Buckets report `CredentialsUnavailable` until the kubelet shows the key files in the pod, which
takes up to a minute after the first start.

The key does not rotate. If the store rejects the key, delete the Secret and restart the pod to
create a new one. The old key stays in the store, which cannot list keys for the operator to
remove it.

## Permissions

The ClusterRole `object-store-operator` is generated from the markers in the Go code with
`make manifests`. Bind it with a RoleBinding in each store's namespace, never with a
ClusterRoleBinding, because it allows creating Secrets. The Deployment runs one replica with the
`Recreate` strategy: there is no leader election, and a second pod would create a second key.

## Open

Where the pod runs (node, host network) is not decided, so the Deployment sets neither.
