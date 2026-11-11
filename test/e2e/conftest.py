import os

import pytest
from kubernetes import client, config as kube_config

import e2e_config

HERE = os.path.dirname(os.path.abspath(__file__))
DEFAULT_CONFIG = os.path.join(HERE, "config", "config.yaml")
REQUEST_TIMEOUT = 15  # seconds; keeps a dead API from hanging the run


def pytest_addoption(parser):
    parser.addoption("--config", default=DEFAULT_CONFIG,
                     help="e2e config file (default: config/config.yaml)")


def pytest_collection_modifyitems(items):
    for item in items:
        if os.sep + "unit" + os.sep in str(item.path):
            item.add_marker(pytest.mark.unit)


@pytest.fixture(scope="session")
def cfg(request):
    path = os.path.abspath(request.config.getoption("--config"))
    try:
        return e2e_config.load(path)
    except (OSError, ValueError) as e:
        pytest.fail(f"invalid e2e config {path}: {e}", pytrace=False)


@pytest.fixture(scope="session")
def api_client(cfg):
    """ApiClient bound to the configured kubeconfig and context.

    Checks the API is reachable before any test talks to the cluster.
    """
    ctx = cfg.cluster.context
    try:
        conf = client.Configuration()
        kube_config.load_kube_config(config_file=e2e_config.kubeconfig_path(cfg),
                                     context=ctx, client_configuration=conf)
        conf.retries = 0  # urllib3 default would multiply the timeout on a dead API
        api = client.ApiClient(conf)
    except (OSError, kube_config.ConfigException) as e:
        pytest.fail(f"cannot use context {ctx!r} from {e2e_config.kubeconfig_path(cfg)}: {e}",
                    pytrace=False)
    try:
        client.VersionApi(api).get_code(_request_timeout=REQUEST_TIMEOUT)
    except Exception as e:
        pytest.fail(f"API server for context {ctx!r} is unreachable ({type(e).__name__}). "
                    "Is the cluster up? For kind run `make local-dev`.", pytrace=False)
    return api


@pytest.fixture(scope="session")
def core_v1(api_client):
    return client.CoreV1Api(api_client)


@pytest.fixture(scope="session")
def apps_v1(api_client):
    return client.AppsV1Api(api_client)


@pytest.fixture(scope="session")
def coordination_v1(api_client):
    return client.CoordinationV1Api(api_client)
