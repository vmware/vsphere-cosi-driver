"""A bucket is created, kept or removed on the store as its ObjectStoreBucket asks."""
import pytest
from kubernetes.client.rest import ApiException

import e2e_buckets as buckets
import e2e_config
import e2e_wait
from standins.s3_backend import fake as s3_backend

pytestmark = pytest.mark.needs("s3_backend", "iam", "objectstore", "admin_secret")

# The operator's key reaches the pod through a Secret volume, which the kubelet refreshes about
# once a minute, so the first bucket is slow.
FIRST_READY_TIMEOUT = 240


def test_operator_bootstraps_its_credentials(store, core_v1):
    """The operator creates its own key with the store's root credentials and keeps it in a Secret."""
    found = []

    def check():
        try:
            found[:] = [core_v1.read_namespaced_secret(e2e_config.OPERATOR_SECRET, store)]
        except ApiException as e:
            if e.status == 404:
                return False, f"Secret {store}/{e2e_config.OPERATOR_SECRET} does not exist"
            raise
        return True, ""

    e2e_wait.until(check, f"Secret {e2e_config.OPERATOR_SECRET}", timeout=FIRST_READY_TIMEOUT)
    secret = found[0]
    root = core_v1.read_namespaced_secret(e2e_config.ROOT_SECRET, store)
    assert secret.data["akey"] and secret.data["secret"]
    assert secret.data["akey"] != root.data["akey"], "the operator must not keep the root key"
    assert [r.kind for r in secret.metadata.owner_references] == ["ObjectStore"]


def test_create_becomes_ready(store, core_v1, custom_v1, bucket_names):
    name = buckets.unique_name("e2e-create")
    bucket_names.append(name)

    buckets.create(custom_v1, store, name, "Delete")
    bucket = buckets.wait_ready(custom_v1, store, name, timeout=FIRST_READY_TIMEOUT)

    assert bucket["status"]["bucketID"] == name
    assert buckets.ready_condition(bucket)["reason"] == "Provisioned"
    assert s3_backend.bucket_exists(core_v1, name)


def test_recreate_is_idempotent(store, core_v1, custom_v1, bucket_names):
    name = buckets.unique_name("e2e-recreate")
    bucket_names.append(name)
    buckets.create(custom_v1, store, name, "Retain")
    buckets.wait_ready(custom_v1, store, name, timeout=FIRST_READY_TIMEOUT)
    buckets.delete(custom_v1, store, name)
    buckets.wait_gone(custom_v1, store, name)

    buckets.create(custom_v1, store, name, "Retain")
    bucket = buckets.wait_ready(custom_v1, store, name)

    assert buckets.ready_condition(bucket)["reason"] == "Provisioned"
    assert s3_backend.bucket_exists(core_v1, name)


def test_delete_removes_bucket(store, core_v1, custom_v1, bucket_names):
    name = buckets.unique_name("e2e-delete")
    bucket_names.append(name)
    buckets.create(custom_v1, store, name, "Delete")
    buckets.wait_ready(custom_v1, store, name, timeout=FIRST_READY_TIMEOUT)

    buckets.delete(custom_v1, store, name)
    buckets.wait_gone(custom_v1, store, name)

    assert not s3_backend.bucket_exists(core_v1, name)


def test_delete_retain_keeps_bucket(store, core_v1, custom_v1, bucket_names):
    name = buckets.unique_name("e2e-retain")
    bucket_names.append(name)
    buckets.create(custom_v1, store, name, "Retain")
    buckets.wait_ready(custom_v1, store, name, timeout=FIRST_READY_TIMEOUT)

    buckets.delete(custom_v1, store, name)
    buckets.wait_gone(custom_v1, store, name)

    assert s3_backend.bucket_exists(core_v1, name)
