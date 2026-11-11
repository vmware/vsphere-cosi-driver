"""Checks the testbed is ready: cluster reachable, driver and controller up, driver leader lease live."""
import pytest
from kubernetes import client
from kubernetes.client.rest import ApiException

import e2e_wait

FIX = ("Deploy the driver first (local testbed: `make local-dev`; real "
       "Supervisor: your own deployment) and check the names in the --config file.")


def _forbidden(e, what):
    """Fails with the missing permission when the API refuses a read; otherwise returns."""
    if e.status in (401, 403):
        pytest.fail(f"Not allowed to read {what} (HTTP {e.status}). The suite needs read "
                    "access to Deployments in the driver and controller namespaces and to "
                    "Leases in the driver namespace.", pytrace=False)


def _deployment_ready(apps_v1, namespace, name):
    what = f"Deployment {namespace}/{name}"
    try:
        d = apps_v1.read_namespaced_deployment(name, namespace)
    except ApiException as e:
        _forbidden(e, what)
        if e.status == 404:  # may be mid-apply: keep polling until the timeout
            return False, f"{what} not found"
        raise
    want = d.spec.replicas or 0
    status = d.status
    available = status.available_replicas or 0
    updated = status.updated_replicas or 0
    # available_replicas also counts pods of the previous ReplicaSet, so a rollout is only
    # done once the controller has seen this spec and every replica runs it.
    observed = (status.observed_generation or 0) >= (d.metadata.generation or 0)
    return (observed and updated == want and available >= want > 0,
            f"{available}/{want} replicas available, {updated}/{want} updated"
            + ("" if observed else ", spec change not yet observed"))


def _wait_deployment(apps_v1, namespace, name):
    try:
        e2e_wait.until(lambda: _deployment_ready(apps_v1, namespace, name),
                       f"Deployment {namespace}/{name} to be Available")
    except TimeoutError as e:
        pytest.fail(f"{e}. {FIX}", pytrace=False)


def test_cluster_reachable(api_client):
    """The configured context answers; the api_client fixture fails otherwise."""
    version = client.VersionApi(api_client).get_code(_request_timeout=15)
    assert version.git_version


def test_driver_deployment_available(apps_v1, cfg):
    """The driver Deployment has all its replicas Available."""
    _wait_deployment(apps_v1, cfg.driver.namespace, cfg.driver.deployment)


def test_controller_deployment_available(apps_v1, cfg):
    """The upstream COSI controller Deployment has all its replicas Available."""
    _wait_deployment(apps_v1, cfg.controller.namespace, cfg.controller.deployment)


def test_driver_leader_lease_live(coordination_v1, cfg):
    """The sidecar holds, and keeps renewing, its leader Lease for the driver.

    This shows the sidecar won leader election for this driver name. It does not show that
    the driver answered DriverGetInfo. Liveness is the Lease's renewTime advancing between
    two reads, so the local clock is not compared with the cluster's.
    """
    name = f"cosi-sidecar-leader-{cfg.driver.name}"
    ns = cfg.driver.namespace
    first_seen = []

    def check():
        try:
            lease = coordination_v1.read_namespaced_lease(name, ns)
        except ApiException as e:
            _forbidden(e, f"Lease {ns}/{name}")
            if e.status == 404:
                return False, f"Lease {ns}/{name} does not exist"
            raise
        holder = lease.spec.holder_identity
        renewed = lease.spec.renew_time
        if not holder or renewed is None:
            return False, f"Lease {ns}/{name} has no holder"
        if not first_seen:
            first_seen.append(renewed)
            return False, f"Lease {ns}/{name} held by {holder}; waiting for a renewal"
        if renewed > first_seen[0]:
            return True, ""
        return False, f"Lease {ns}/{name} held by {holder} is not being renewed"

    try:
        e2e_wait.until(check, f"sidecar Lease {name}", timeout=30)
    except TimeoutError as e:
        pytest.fail(f"{e}. Check that driver.name matches the driver's registered name "
                    "and that the sidecar container is running.", pytrace=False)
