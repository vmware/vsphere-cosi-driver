# Root credentials Secret

Stands in for the Secret `vboss-admin-secret`, which holds the store's root key. Only the operator's bootstrap reads it, once, to create the operator's own key.

**Mode.** With `standins.admin_secret: fake`, the testbed creates the Secret next to the ObjectStore (`hack/kind/operator/store.yaml`) with keys `akey` and `secret`, holding the pair the fake IAM service accepts. The suite reads it to check that the operator's key is not the root key; it never creates it.

**Contract.** A Secret with keys `akey` and `secret` in the store's namespace.

**What it cannot prove.**

- That the real Secret has this name and these keys. They follow the design and are not yet checked against a real Supervisor.
- Who creates the real Secret and when, or what the real root key may do.
- Root key rotation. The operator reads the Secret on every attempt, but no scenario rotates it.
- That the keys are secret: they are fixed and local-only.

**Real mode.** Not supported yet: a scenario that needs the fake is skipped when `standins.admin_secret` is `real` or `off`.
