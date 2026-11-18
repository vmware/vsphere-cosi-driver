"""Fixtures for the operator scenarios."""
import pytest
from kubernetes.client.rest import ApiException

import e2e_buckets
import e2e_wait

STORE_GROUP = "objectstore.vsan.broadcom.com"
STORE_VERSION = "v1alpha1"
STORE_PLURAL = "objectstores"

FIX = "Deploy the testbed first (local testbed: `make local-dev`)."


def _ready_stores(custom_v1, namespace):
    try:
        items = custom_v1.list_namespaced_custom_object(STORE_GROUP, STORE_VERSION, namespace, STORE_PLURAL)["items"]
    except ApiException as e:
        if e.status == 404:  # the CRD is not installed
            return False, f"the {STORE_PLURAL}.{STORE_GROUP} CRD is not installed"
        raise
    if len(items) != 1:
        return False, f"{len(items)} ObjectStore objects in namespace {namespace}, want 1"
    state = (items[0].get("status") or {}).get("healthState")
    return state == "Ready", f"ObjectStore {items[0]['metadata']['name']} is {state!r}, want 'Ready'"


@pytest.fixture(scope="session")
def store(cfg, custom_v1):
    """The operator's namespace, once the testbed's store in it is Ready."""
    ns = cfg.operator.namespace
    try:
        e2e_wait.until(lambda: _ready_stores(custom_v1, ns), f"the ObjectStore in {ns} to be Ready", timeout=30)
    except TimeoutError as e:
        pytest.fail(f"{e}. {FIX}", pytrace=False)
    return ns


@pytest.fixture
def bucket_names(core_v1, custom_v1, store):
    """Names a test registers so that its buckets are removed afterwards, even when it fails."""
    names = []
    yield names
    for name in names:
        e2e_buckets.discard(core_v1, custom_v1, store, name)
