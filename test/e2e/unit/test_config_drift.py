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


def _manifest(*parts):
    with open(os.path.join(ROOT, *parts)) as f:
        return list(yaml.safe_load_all(f))


def _by_kind(docs, kind):
    return next(d for d in docs if d and d.get("kind") == kind)


def test_operator_names_match_manifests_and_deploy_script():
    deploy = _by_kind(_manifest("manifests", "supervisor", "operator", "deployment.yaml"), "Deployment")
    op = e2e_config.Operator()
    assert op.deployment == deploy["metadata"]["name"]
    assert op.deployment == _script_var("OPERATOR_DEPLOY")
    assert op.namespace == _script_var("OPERATOR_NS")


def test_operator_secret_matches_go_constant():
    go = re.search(r'SecretName\s*=\s*"([^"]+)"', _read("internal", "operator", "credentials", "bootstrap.go"))
    assert go, "credentials.SecretName not found"
    assert e2e_config.OPERATOR_SECRET == go.group(1)


def test_fake_iam_service_matches_its_manifest():
    from standins.iam import fake as iam

    docs = _manifest("hack", "kind", "operator", "fake-iam.yaml")
    service = _by_kind(docs, "Service")
    assert service["metadata"]["name"] == "fake-iam"
    assert service["spec"]["ports"][0]["port"] == iam.PORT
    assert _script_var("FAKE_IAM_SCRIPT_CONFIGMAP") == _by_kind(docs, "Deployment")["spec"]["template"][
        "spec"]["volumes"][0]["configMap"]["name"]


def test_minio_stand_in_matches_its_manifest():
    from standins.iam import fake as iam
    from standins.s3_backend import fake as s3_backend

    docs = _manifest("hack", "kind", "minio.yaml")
    service = _by_kind(docs, "Service")
    assert s3_backend.NAMESPACE == service["metadata"]["namespace"]
    env = {e["name"]: e["value"] for e in _by_kind(docs, "Deployment")["spec"]["template"]["spec"]["containers"][0]["env"]}
    assert (iam.ISSUED_ACCESS_KEY, iam.ISSUED_SECRET_KEY) == (env["MINIO_ROOT_USER"], env["MINIO_ROOT_PASSWORD"])


def test_store_fixture_points_at_the_stand_ins():
    from standins.iam import fake as iam

    store = _by_kind(_manifest("hack", "kind", "operator", "store.yaml"), "ObjectStore")
    minio = _by_kind(_manifest("hack", "kind", "minio.yaml"), "Service")
    host = f"{minio['metadata']['name']}.{minio['metadata']['namespace']}.svc.cluster.local"
    assert store["status"]["externalS3Endpoint"]["fqdn"] == f"http://{host}:{minio['spec']['ports'][0]['port']}"
    assert store["status"]["iamEndpoint"]["fqdn"] == f"http://fake-iam:{iam.PORT}"
    assert store["status"]["healthState"] == "Ready"


def test_root_secret_fixture_holds_the_key_the_fake_iam_accepts():
    from standins.iam import fake as iam

    docs = _manifest("hack", "kind", "operator", "store.yaml")
    secret = _by_kind(docs, "Secret")
    assert secret["metadata"]["name"] == e2e_config.ROOT_SECRET
    assert secret["stringData"] == {"akey": iam.ADMIN_ACCESS_KEY, "secret": iam.ADMIN_SECRET_KEY}


def test_root_secret_name_matches_the_operator_manifest():
    deploy = _by_kind(_manifest("manifests", "supervisor", "operator", "deployment.yaml"), "Deployment")
    secrets = [v["secret"]["secretName"] for v in deploy["spec"]["template"]["spec"]["volumes"]]
    assert e2e_config.ROOT_SECRET in secrets
    assert e2e_config.OPERATOR_SECRET in secrets
