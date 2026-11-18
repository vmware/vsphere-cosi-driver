# e2e suite

Integration tests for the vSphere COSI driver. Plain pytest: fixtures in `conftest.py`, marks in `pytest.ini`.

## Prerequisites

- Python 3.10 or newer, with `pip`.
- A testbed with the driver and the upstream COSI controller deployed. The suite verifies it and never creates it. For kind, `make local-dev` creates one; it needs `docker`, `kind`, `kubectl`, `git`, `make`, `go` and the upstream COSI repo next to this one (or its path through `ARGS="-r <path>"`).
- A kubeconfig that has the context named by `cluster.context` in the config. The current context is never used.
- Read access to Deployments in the driver, controller and operator namespaces, to Leases in the driver namespace and, for the scenarios, to Secrets, ObjectStores and ObjectStoreBuckets in the operator's namespace, plus `pods/exec` in the MinIO namespace (the `s3_backend` stand-in reads the backend through the MinIO pod). Scenarios also create and delete ObjectStoreBuckets there.

## Run

```bash
pip install -r test/e2e/requirements.txt         # once
make test-e2e                                    # uses config/config.yaml (default: the local kind testbed)
make test-e2e CONFIG=my-config.yaml              # your own cluster
make test-e2e ARGS="-k lease"               # any pytest arguments
pytest test/e2e --config my-config.yaml          # make is optional
make test-e2e-unit                               # the suite's own tests; no cluster needed
```

CI: add `--junitxml=report.xml` through `ARGS`.

## Config

One YAML file; see `config/config.yaml` and `config/examples/`. Unknown keys are errors, `cluster.context` is required and is the only context used. `operator` names the operator's namespace and Deployment. `standins` sets the mode of each stand-in (see below). `--config` only selects the file; nothing else overrides a setting. To test a different driver build, deploy it first (local testbed: `git checkout <ref> && make local-dev`); the suite tests whatever is deployed.

For Supervisor, copy `config/examples/supervisor.yaml`, set `cluster.context` and any names that differ in your deployment, and pass it with `CONFIG`. Its names follow the design doc and are not yet verified against a real Supervisor. Examples for other cluster types will be added as they are supported.

You prepare the testbed. The suite verifies it; it never provisions it. The object store operator is part of it, with its stand-ins (see Stand-ins). `hack/deploy-kind-cluster.sh` is the helper for a local kind testbed. Kind is only where the tests run: it deploys the Supervisor layout.

### Local testbed (kind)

`make local-dev` runs `hack/deploy-kind-cluster.sh`; `hack/deploy-kind-cluster.sh -h` lists its flags and `make local-dev ARGS="-c other"` passes them through. It creates the `cosi-cluster` kind cluster if needed, builds the driver for the node's architecture, and deploys the Supervisor layout: the upstream controller as `cosi-controller` in `kube-system`, and the driver with its sidecar from `manifests/supervisor`, adapted to one node by the overlay in `hack/kind/` (one replica, no pod anti-affinity, `ClusterFirstWithHostNet` DNS so the `hostNetwork` pod can resolve MinIO). An in-cluster MinIO stands in for the object store. The script also deploys the object store operator in the `cosi-store` namespace, with the two CRDs it needs, a fake IAM service, and an ObjectStore with its root credentials Secret. It builds the operator for the node's architecture (`images/operator/Dockerfile.dev`) and loads a pinned Python image for the fake IAM service (`FAKE_IAM_IMAGE`, `FAKE_IAM_IMAGE_TAR`, like MinIO). The operator creates its own key at startup; the Secret volume that carries it reaches the pod about a minute later, so the first bucket after a fresh deploy takes about that long. Re-running it rebuilds only what changed. A single-node cluster cannot exercise multi-replica behaviour.

- The upstream COSI repo (`kubernetes-sigs/container-object-storage-interface`) is expected next to this one, or pass its path with `ARGS="-r <path>"`.
- MinIO is pinned to a release tag of `ghcr.io/coollabsio/minio`. Set `MINIO_IMAGE` to use another tagged image. If the registry is unreachable, `docker save` the image elsewhere and point `MINIO_IMAGE_TAR` at the tarball; the script loads it.
- `make local-dev-teardown` removes the operator namespace, the operator's ClusterRole and both CRDs, then the driver namespace (including the backend Secret), the MinIO namespace and the driver's ClusterRole and binding. The cluster and the upstream controller stay. Add `ARGS="--delete-cluster"` to delete the whole cluster, which also removes any other driver deployed on it.

## Stand-ins

Every external service the operator touches is a stand-in with a mode in the config: `fake` (the testbed provides it), `real` (the service exists) or `off`. A scenario declares what it needs with `@pytest.mark.needs("iam", ...)` and is skipped, with the reason, unless each of them is `fake`. Real mode has no scenarios yet.

| Stand-in       | Replaces                               | Provided by                                          |
|----------------|----------------------------------------|------------------------------------------------------|
| `s3_backend`   | the store's S3 service                 | MinIO (`hack/kind/minio.yaml`), read through its pod |
| `iam`          | the store's IAM service                | `standins/iam/fake.py` in the cluster                |
| `objectstore`  | the vSAN `ObjectStore` custom resource | a fixture CRD and object (`hack/kind/operator`)      |
| `admin_secret` | the store's root credentials Secret    | a fixture Secret (`hack/kind/operator`)              |

Each directory in `standins/` has a README with the contract the stand-in keeps and what it cannot prove.

## Tests

`test_prerequisites.py` fails fast, naming the fix, when the testbed is not ready. `scenarios/` holds the operator's end-to-end scenarios: it bootstraps its key, creates a bucket and keeps or removes it as the deletion policy says. Scenarios create ObjectStoreBuckets with unique names and remove them, and their backend buckets, afterwards. State is read from the Kubernetes API and the S3 backend, never from log lines.

If kind's API port changed (Docker restart), run `kind export kubeconfig --name cosi-cluster`.
