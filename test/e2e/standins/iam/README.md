# Fake IAM service

Stands in for the object store's IAM endpoint (`status.iamEndpoint` of the ObjectStore). Kind has no object store, and MinIO serves no IAM API.

**Mode.** With `standins.iam: fake`, the testbed runs `fake.py` as Service `fake-iam` in the operator's namespace (`hack/kind/operator`). `hack/deploy-kind-cluster.sh` hands the script to a stock Python image through a ConfigMap.

**Contract.** It answers the four calls the operator makes, in the AWS IAM query protocol:

| Call              | Answer                                                    |
|-------------------|-----------------------------------------------------------|
| `CreateUser`      | 200, or 409 `EntityAlreadyExists` when the user exists    |
| `PutUserPolicy`   | 200, or 404 `NoSuchEntity` for an unknown user            |
| `CreateAccessKey` | 200 with a key, or 404 `NoSuchEntity` for an unknown user |
| `DeleteAccessKey` | 200, or 404 `NoSuchEntity` when the key is not the user's |

Only the store's root key may call it (403 `AccessDenied` otherwise). Any other action is 400 `InvalidAction`.

**What it cannot prove.**

- That the real store accepts or enforces the policy the operator writes. The fake keeps it and ignores it.
- That keys are distinct. Every key it issues is MinIO's root pair, because MinIO knows no other user, so a second `CreateAccessKey` returns the same id.
- Request signatures. It reads the key id from the `Authorization` header and checks nothing else.
- Key limits, throttling, TLS, or any error the real store may return.

**Real mode.** Not supported yet: a scenario that needs the fake is skipped when `standins.iam` is `real` or `off`. The Go suite `internal/iam/contract` holds what the operator needs from a store's IAM service. There is no Python contract test that runs against a real store.
