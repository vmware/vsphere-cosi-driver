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


def test_operator_defaults(tmp_path):
    cfg = e2e_config.load(write(tmp_path, {"cluster": {"context": "x"}}))
    assert (cfg.operator.namespace, cfg.operator.deployment) == ("cosi-store", "object-store-operator")


def test_standins_default_to_fake(tmp_path):
    cfg = e2e_config.load(write(tmp_path, {"cluster": {"context": "x"}}))
    assert cfg.standins == e2e_config.Standins(s3_backend="fake", iam="fake", objectstore="fake",
                                               admin_secret="fake")


def test_standins_mode_accepted(tmp_path):
    cfg = e2e_config.load(write(tmp_path, {"cluster": {"context": "x"},
                                           "standins": {"iam": "real", "objectstore": "off"}}))
    assert (cfg.standins.iam, cfg.standins.objectstore) == ("real", "off")


def test_standins_rejects_unknown_mode(tmp_path):
    p = write(tmp_path, {"cluster": {"context": "x"}, "standins": {"iam": "mock"}})
    with pytest.raises(ValueError, match="standins.iam.*mock"):
        e2e_config.load(p)


def test_standins_rejects_unknown_stand_in(tmp_path):
    p = write(tmp_path, {"cluster": {"context": "x"}, "standins": {"wcpsvc": "fake"}})
    with pytest.raises(ValueError, match="wcpsvc"):
        e2e_config.load(p)
