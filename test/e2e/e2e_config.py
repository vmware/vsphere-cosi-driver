"""Strict loader for the e2e test config."""
import os
from dataclasses import dataclass, fields

import yaml

@dataclass(frozen=True)
class Cluster:
    context: str
    kubeconfig: str = "~/.kube/config"


@dataclass(frozen=True)
class Driver:
    name: str = "cosi.vsphere.vmware.com"
    namespace: str = "vmware-system-cosi"
    deployment: str = "vsphere-cosi-controller"


@dataclass(frozen=True)
class Controller:
    namespace: str = "kube-system"
    deployment: str = "cosi-controller"


# The Secret in which the operator keeps its own key (credentials.SecretName in the Go code).
OPERATOR_SECRET = "cosi-operator-credentials"

# The Secret with the store's root credentials, which only the operator's bootstrap reads.
ROOT_SECRET = "vboss-admin-secret"


@dataclass(frozen=True)
class Operator:
    namespace: str = "cosi-store"
    deployment: str = "object-store-operator"


# fake: the suite sets the stand-in up and tests against it. real: the service exists in the
# cluster already. off: the service is not available.
MODES = ("fake", "real", "off")


@dataclass(frozen=True)
class Standins:
    s3_backend: str = "fake"
    iam: str = "fake"
    objectstore: str = "fake"
    admin_secret: str = "fake"

    def __post_init__(self):
        for f in fields(self):
            mode = getattr(self, f.name)
            if mode not in MODES:
                raise ValueError(f"standins.{f.name}: unknown mode {mode!r}; allowed: {list(MODES)}")


def unmet_standins(standins, names):
    """The named stand-ins that are not configured as fake, as {name: mode}."""
    known = {f.name for f in fields(Standins)}
    unknown = sorted(set(names) - known)
    if unknown:
        raise ValueError(f"needs(): unknown stand-in(s) {unknown}; allowed: {sorted(known)}")
    return {n: getattr(standins, n) for n in names if getattr(standins, n) != "fake"}


@dataclass(frozen=True)
class Config:
    cluster: Cluster
    driver: Driver
    controller: Controller
    operator: Operator
    standins: Standins


def _section(raw, name, cls):
    data = raw.get(name) or {}
    if not isinstance(data, dict):
        raise ValueError(f"{name}: expected a mapping")
    known = {f.name for f in fields(cls)}
    unknown = sorted(set(data) - known)
    if unknown:
        raise ValueError(f"{name}: unknown key(s) {unknown}; allowed: {sorted(known)}")
    return cls(**data)


def load(path):
    with open(path) as f:
        raw = yaml.safe_load(f) or {}
    if not isinstance(raw, dict):
        raise ValueError(f"{path}: expected a mapping at the top level")
    unknown = sorted(set(raw) - {"cluster", "driver", "controller", "operator", "standins"})
    if unknown:
        raise ValueError(f"unknown top-level key(s) {unknown}")
    if not (raw.get("cluster") or {}).get("context"):
        raise ValueError("cluster.context is required (the suite never uses the current context)")
    cfg = Config(
        cluster=_section(raw, "cluster", Cluster),
        driver=_section(raw, "driver", Driver),
        controller=_section(raw, "controller", Controller),
        operator=_section(raw, "operator", Operator),
        standins=_section(raw, "standins", Standins),
    )
    return cfg


def kubeconfig_path(cfg):
    return os.path.expanduser(cfg.cluster.kubeconfig)
