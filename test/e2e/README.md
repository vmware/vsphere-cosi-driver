# e2e suite

Integration tests for the vSphere COSI driver. Plain pytest: fixtures in `conftest.py`, marks in `pytest.ini`.

## Prerequisites

- Python 3.10 or newer, with `pip`.
- A testbed with the driver and the upstream COSI controller deployed. The suite verifies it and never creates it. For kind, `make local-dev` creates one; it needs `docker`, `kind`, `kubectl`, `git`, `make`, `go` and the upstream COSI repo next to this one (or its path through `ARGS="-r <path>"`).
- A kubeconfig that has the context named by `cluster.context` in the config. The current context is never used.
- Read access to Deployments in the driver and controller namespaces and to Leases in the driver namespace.

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

One YAML file; see `config/config.yaml` and `config/examples/`. Unknown keys are errors, `cluster.context` is required and is the only context used. `--config` only selects the file; nothing else overrides a setting. To test a different driver build, deploy it first (local testbed: `git checkout <ref> && make local-dev`); the suite tests whatever is deployed.

For Supervisor, copy `config/examples/supervisor.yaml`, set `cluster.context` and any names that differ in your deployment, and pass it with `CONFIG`. Its names follow the design doc and are not yet verified against a real Supervisor. Examples for other cluster types will be added as they are supported.

You prepare the testbed. The suite verifies it; it never provisions it. `hack/deploy-kind-cluster.sh` is the helper for a local kind testbed. Kind is only where the tests run: it deploys the Supervisor layout.

### Local testbed (kind)

`make local-dev` runs `hack/deploy-kind-cluster.sh`; `hack/deploy-kind-cluster.sh -h` lists its flags and `make local-dev ARGS="-c other"` passes them through. It creates the `cosi-cluster` kind cluster if needed, builds the driver for the node's architecture, and deploys the Supervisor layout: the upstream controller as `cosi-controller` in `kube-system`, and the driver with its sidecar from `manifests/supervisor`, adapted to one node by the overlay in `hack/kind/` (one replica, no pod anti-affinity, `ClusterFirstWithHostNet` DNS so the `hostNetwork` pod can resolve MinIO). An in-cluster MinIO stands in for the object store. Re-running it rebuilds only what changed. A single-node cluster cannot exercise multi-replica behaviour.

- The upstream COSI repo (`kubernetes-sigs/container-object-storage-interface`) is expected next to this one, or pass its path with `ARGS="-r <path>"`.
- MinIO is pinned to a release tag of `ghcr.io/coollabsio/minio`. Set `MINIO_IMAGE` to use another tagged image. If the registry is unreachable, `docker save` the image elsewhere and point `MINIO_IMAGE_TAR` at the tarball; the script loads it.
- `make local-dev-teardown` removes the driver namespace (including the backend Secret), the MinIO namespace and the driver's ClusterRole and binding. The cluster and the upstream controller stay. Add `ARGS="--delete-cluster"` to delete the whole cluster, which also removes any other driver deployed on it.

## Tests

`test_prerequisites.py` fails fast, naming the fix, when the testbed is not ready. Scenario tests land here as the driver's RPCs do. State is read from the Kubernetes API, never from log lines.

If kind's API port changed (Docker restart), run `kind export kubeconfig --name cosi-cluster`.
