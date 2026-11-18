# S3 backend

Stands in for the object store's S3 service: the in-cluster MinIO of `hack/kind/minio.yaml`, reached by the operator through `status.externalS3Endpoint`.

**Mode.** With `standins.s3_backend: fake`, the testbed runs MinIO. `fake.py` reads the backend through the MinIO pod (`ls /data`), so the suite needs no S3 client and no route from the host to the cluster.

**Contract.** `buckets()` returns the names of the buckets that exist, and `bucket_exists(name)` tests one. In single-drive mode MinIO keeps each bucket as a top-level directory of `/data`, next to its hidden metadata directory.

**What it cannot prove.**

- That the vSAN object store behaves like MinIO: bucket name rules, S3 error codes, `maxBuckets`, versioning, locks, TLS and regions.
- A non-empty bucket. The helper only lists, so a scenario cannot put an object with it.
- That the check matches what an S3 client sees. It reads the storage layout, which a MinIO release may change.

**Real mode.** Not supported yet: a scenario that needs the fake is skipped when `standins.s3_backend` is `real` or `off`.
