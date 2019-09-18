# Test cluster


## Namespace Deleter

Some of our installation tests us a shared cluster.
Occasionaly, stale resources are left in the cluster.
When too many stale resources exist, our cluster is unable to schedule new pods.
The `namespace-delter.yaml` CronJob helps mitigate this problem by deleting stale resources on a regular basis.


