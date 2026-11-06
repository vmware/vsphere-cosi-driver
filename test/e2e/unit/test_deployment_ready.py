"""Readiness logic of the prerequisites checks; needs no cluster."""
from types import SimpleNamespace as NS

import pytest
from kubernetes.client.rest import ApiException

import test_prerequisites as prereq


def deployment(replicas=2, available=2, updated=2, generation=3, observed=3):
    return NS(metadata=NS(generation=generation),
              spec=NS(replicas=replicas),
              status=NS(available_replicas=available, updated_replicas=updated,
                        observed_generation=observed))


class FakeApps:
    def __init__(self, result):
        self.result = result

    def read_namespaced_deployment(self, name, namespace):
        if isinstance(self.result, Exception):
            raise self.result
        return self.result


def ready(result):
    return prereq._deployment_ready(FakeApps(result), "ns", "d")


def test_all_replicas_updated_and_available():
    assert ready(deployment())[0] is True


def test_mid_rollout_old_pods_still_available_is_not_ready():
    assert ready(deployment(available=2, updated=1))[0] is False


def test_spec_change_not_yet_observed_is_not_ready():
    done, detail = ready(deployment(generation=4, observed=3))
    assert done is False and "not yet observed" in detail


def test_zero_replicas_is_not_ready():
    assert ready(deployment(replicas=0, available=0, updated=0))[0] is False


def test_missing_deployment_is_retried():
    done, detail = ready(ApiException(status=404))
    assert done is False and "not found" in detail


@pytest.mark.parametrize("status", [401, 403])
def test_forbidden_fails_naming_the_permission(status):
    with pytest.raises(pytest.fail.Exception, match="read access"):
        ready(ApiException(status=status))


def test_other_api_errors_propagate():
    with pytest.raises(ApiException):
        ready(ApiException(status=500))
