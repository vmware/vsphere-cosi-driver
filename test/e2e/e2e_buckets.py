"""ObjectStoreBucket helpers for the scenarios. State is read from the Kubernetes API."""
import uuid

from kubernetes.client.rest import ApiException

import e2e_wait
from standins.s3_backend import fake as s3_backend

GROUP = "cosi.vsphere.vmware.com"
VERSION = "v1alpha1"
PLURAL = "objectstorebuckets"
DRIVER_NAME = "cosi.vsphere.vmware.com"
CLAIM_NAMESPACE = "workload"  # where the claim would live; no claim object exists in these tests


def unique_name(prefix):
    return f"{prefix}-{uuid.uuid4().hex[:8]}"


def build(namespace, name, policy):
    return {
        "apiVersion": f"{GROUP}/{VERSION}",
        "kind": "ObjectStoreBucket",
        "metadata": {"name": name, "namespace": namespace},
        "spec": {
            "driverName": DRIVER_NAME,
            "deletionPolicy": policy,
            "protocols": ["S3"],
            "bucketClaimRef": {"name": f"claim-{name}", "namespace": CLAIM_NAMESPACE},
        },
    }


def create(custom_v1, namespace, name, policy):
    custom_v1.create_namespaced_custom_object(GROUP, VERSION, namespace, PLURAL, build(namespace, name, policy))


def get(custom_v1, namespace, name):
    """The bucket object, or None when it does not exist."""
    try:
        return custom_v1.get_namespaced_custom_object(GROUP, VERSION, namespace, PLURAL, name)
    except ApiException as e:
        if e.status == 404:
            return None
        raise


def ready_condition(bucket):
    """The Ready condition of a bucket object, or None."""
    for c in (bucket.get("status") or {}).get("conditions") or []:
        if c.get("type") == "Ready":
            return c
    return None


def _why(bucket):
    if bucket is None:
        return "the bucket object does not exist"
    cond = ready_condition(bucket)
    if cond is None:
        return "no Ready condition yet"
    return f"Ready is {cond.get('status')}: {cond.get('reason')}: {cond.get('message')}"


def wait_ready(custom_v1, namespace, name, timeout=120):
    def check():
        bucket = get(custom_v1, namespace, name)
        ready = ((bucket or {}).get("status") or {}).get("readyToUse") is True
        return ready, _why(bucket)

    e2e_wait.until(check, f"ObjectStoreBucket {namespace}/{name} to be ready", timeout=timeout)
    return get(custom_v1, namespace, name)


def delete(custom_v1, namespace, name):
    try:
        custom_v1.delete_namespaced_custom_object(GROUP, VERSION, namespace, PLURAL, name)
    except ApiException as e:
        if e.status != 404:
            raise


def wait_gone(custom_v1, namespace, name, timeout=120):
    """Waits for the bucket object to be gone, which is when its finalizer has been removed."""
    e2e_wait.until(lambda: (get(custom_v1, namespace, name) is None, _why(get(custom_v1, namespace, name))),
                   f"ObjectStoreBucket {namespace}/{name} to be deleted", timeout=timeout)


def set_policy(custom_v1, namespace, name, policy):
    custom_v1.patch_namespaced_custom_object(GROUP, VERSION, namespace, PLURAL, name,
                                             {"spec": {"deletionPolicy": policy}})


def discard(core_v1, custom_v1, namespace, name):
    """Removes a bucket object and its backend bucket, whatever state the test left them in."""
    bucket = get(custom_v1, namespace, name)
    if bucket is None and not s3_backend.bucket_exists(core_v1, name):
        return
    if bucket is None:
        create(custom_v1, namespace, name, "Delete")  # adopts the retained backend bucket
        wait_ready(custom_v1, namespace, name)
    else:
        set_policy(custom_v1, namespace, name, "Delete")
    delete(custom_v1, namespace, name)
    wait_gone(custom_v1, namespace, name)
