"""Stand-in for the object store's S3 service: the in-cluster MinIO of hack/kind/minio.yaml.

The suite reads the backend through the MinIO pod, not through S3, so it needs no S3 client and
no route from the host to the cluster.
"""
from kubernetes.stream import stream

NAMESPACE = "minio"
SELECTOR = "app=minio"
CONTAINER = "minio"

# MinIO in single-drive mode keeps each bucket as a top-level directory of its data directory,
# next to its own hidden metadata directory.
DATA_DIR = "/data"


def parse(listing):
    """The bucket names in the output of `ls -1` on the data directory."""
    return {line for line in listing.splitlines() if line and not line.startswith(".")}


def buckets(core_v1):
    pods = core_v1.list_namespaced_pod(NAMESPACE, label_selector=SELECTOR).items
    running = [p.metadata.name for p in pods if p.status.phase == "Running"]
    if not running:
        raise RuntimeError(f"no running MinIO pod with label {SELECTOR} in namespace {NAMESPACE}")
    out = stream(core_v1.connect_get_namespaced_pod_exec, running[0], NAMESPACE, container=CONTAINER,
                 command=["ls", "-1", DATA_DIR], stdin=False, stdout=True, stderr=True, tty=False)
    return parse(out)


def bucket_exists(core_v1, name):
    return name in buckets(core_v1)
