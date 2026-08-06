# transfer-operator

> [!WARNING]
> transfer-operator is an experimental operator and has little to no
> safeguards against eternal reconcile loops.
> Its APIs and interfaces may change at any time.

Mirrors Kubernetes objects between a control plane and a fleet of remote clusters.

Two APIs in the `transfer.platform-mesh.io` group, both cluster-scoped:

- **`KubeconfigProvider`** — engages a set of remote clusters from kubeconfig Secrets.
  The object's name is the provider name.
- **`Transfer`** — which resources move, in which direction, and to the clusters of which provider.

`Transfer.spec.direction` decides which side owns the spec:

- `Push` sends the spec to the remote cluster and reflects the remote status back
- `Pull` copies the spec from the remote cluster and reflects the status back

## Modes

Against a **single control plane** both CRDs live in the local cluster.

Against **kcp** the operator serves the `transfer.platform-mesh.io`
APIExport and reads the configuration out of every workspace that bound
it. Payload objects are reached with an admin kubeconfig rather than
through the APIExport virtual workspace, because a `Transfer` may select
any API group and permission claims are static and per-resource.

## Development

```
task build-transfer-operator
task test-transfer-operator
task verify-transfer-operator
```
