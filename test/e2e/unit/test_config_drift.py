"""Guards the names that are repeated across the Go code, the manifests, the deploy script and
the shipped config. No cluster needed."""
import os
import re

import yaml

import e2e_config

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", ".."))
SHIPPED = os.path.join(ROOT, "test", "e2e", "config", "config.yaml")


def _read(*parts):
    with open(os.path.join(ROOT, *parts)) as f:
        return f.read()


def _script_var(name):
    match = re.search(rf'^{name}="([^"]+)"', _read("hack", "deploy-kind-cluster.sh"), re.M)
    assert match, f"{name} not found in hack/deploy-kind-cluster.sh"
    return match.group(1)


def _supervisor_deployment():
    docs = yaml.safe_load_all(_read("manifests", "supervisor", "cosi.yaml"))
    return next(d for d in docs if d and d.get("kind") == "Deployment")


def test_default_driver_name_matches_go_constant():
    go = re.search(r'Name\s*=\s*"([^"]+)"', _read("pkg", "cosi", "types", "constants.go"))
    assert go, "types.Name not found"
    assert e2e_config.Driver().name == go.group(1)


def test_shipped_config_driver_matches_supervisor_manifest():
    deploy = _supervisor_deployment()
    cfg = e2e_config.load(SHIPPED)
    assert cfg.driver.namespace == deploy["metadata"]["namespace"]
    assert cfg.driver.deployment == deploy["metadata"]["name"]


def test_shipped_config_matches_deploy_script():
    cfg = e2e_config.load(SHIPPED)
    assert cfg.driver.namespace == _script_var("DRIVER_NS")
    assert cfg.driver.deployment == _script_var("DRIVER_DEPLOY")
    assert cfg.controller.namespace == _script_var("CONTROLLER_NS")
    assert cfg.controller.deployment == _script_var("CONTROLLER_DEPLOY")
