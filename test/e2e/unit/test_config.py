"""Loader tests; need no cluster. Run alone with `pytest test/e2e/unit`."""
import os

import pytest
import yaml

import e2e_config


SHIPPED = os.path.join(os.path.dirname(__file__), "..", "config", "config.yaml")


def write(tmp_path, data):
    p = tmp_path / "c.yaml"
    p.write_text(yaml.safe_dump(data))
    return p


def test_unknown_key_rejected(tmp_path):
    p = write(tmp_path, {"cluster": {"context": "x", "bogus": 1}})
    with pytest.raises(ValueError, match="bogus"):
        e2e_config.load(p)


def test_unknown_top_level_key_rejected(tmp_path):
    p = write(tmp_path, {"cluster": {"context": "x"}, "vcenter": {}})
    with pytest.raises(ValueError, match="vcenter"):
        e2e_config.load(p)


def test_missing_context_rejected(tmp_path):
    with pytest.raises(ValueError, match="cluster.context"):
        e2e_config.load(write(tmp_path, {"cluster": {"kubeconfig": "~/.kube/config"}}))


def test_flavor_key_rejected(tmp_path):
    with pytest.raises(ValueError, match="flavor"):
        e2e_config.load(write(tmp_path, {"cluster": {"context": "x", "flavor": "supervisor"}}))


def test_named_context_used(tmp_path):
    cfg = e2e_config.load(write(tmp_path, {"cluster": {"context": "my-ctx"}}))
    assert cfg.cluster.context == "my-ctx"


def test_shipped_config_loads():
    assert e2e_config.load(SHIPPED).cluster.context == "kind-cosi-cluster"


@pytest.mark.parametrize("name", sorted(os.listdir(os.path.join(os.path.dirname(SHIPPED), "examples"))))
def test_example_configs_load(name):
    path = os.path.join(os.path.dirname(SHIPPED), "examples", name)
    assert e2e_config.load(path).cluster.context
