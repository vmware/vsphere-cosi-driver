# ObjectStore

Stands in for the vSAN `ObjectStore` custom resource, which the vSAN team owns and the operator only reads. Kind has no such CRD.

**Mode.** With `standins.objectstore: fake`, the testbed installs a fixture CRD (`hack/kind/operator/objectstore-crd.yaml`) and one object (`hack/kind/operator/store.yaml`) in the operator's namespace. The suite checks that the object exists and reports `Ready`; it never creates it.

**Contract.** The object reports `status.healthState: Ready`, `status.externalS3Endpoint.fqdn` (MinIO) and `status.iamEndpoint.fqdn` (the fake IAM service). These are the fields the operator reads (`internal/vboss/v1alpha1`).

**What it cannot prove.**

- That the real CRD has these field names and shapes. The fixture was written from the field contract and keeps unknown fields.
- Any other health state. The fixture is always `Ready`, so store-not-ready handling needs a scenario that edits the object.
- TLS (`spec.enableTLS`, `certSecretRef`), `spec.domain`, `spec.maxBuckets` and conditions. The operator does not read them yet.
- That the real store publishes its endpoints as full URLs.

**Real mode.** Not supported yet: a scenario that needs the fake is skipped when `standins.objectstore` is `real` or `off`.
