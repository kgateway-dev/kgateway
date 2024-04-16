# Actions

A [ClusterAction](./action.go) is a function that will be executed against the cluster to mutate its state.

Actions are intended to mirror actions that users of the product can take. We group these actions in a package associated with the tools that users would rely on (`glooctl`, `kubectl`..etc).

If you intend to introduce a new action, please follow this approach:
- We want to avoid writing custom code that is _just_ used by our tests. The core action should live in a utility package ([utils](/pkg/utils), [cli-utils](/pkg/cliutil), ...etc)
- The `ClusterAction` that you add should live in the actions package associated with that tool

**Example**:
If you expanded the functionality that a user would perform via `kubectl`
1. Introduce that logic into the [kubectl](/pkg/utils/kubeutils/kubectl) package
2. Expose that as a `ClusterAction` in the [kubectl](./kubectl) actions package