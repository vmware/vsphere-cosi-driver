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


@dataclass(frozen=True)
class Config:
    cluster: Cluster
    driver: Driver
    controller: Controller


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
    unknown = sorted(set(raw) - {"cluster", "driver", "controller"})
    if unknown:
        raise ValueError(f"unknown top-level key(s) {unknown}")
    if not (raw.get("cluster") or {}).get("context"):
        raise ValueError("cluster.context is required (the suite never uses the current context)")
    cfg = Config(
        cluster=_section(raw, "cluster", Cluster),
        driver=_section(raw, "driver", Driver),
        controller=_section(raw, "controller", Controller),
    )
    return cfg


def kubeconfig_path(cfg):
    return os.path.expanduser(cfg.cluster.kubeconfig)
