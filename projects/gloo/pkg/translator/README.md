# Gloo Translation

The Gloo Translator is responsible for converting a Gloo Proxy into an xDS Snapshot. It does this in the following order:

1. Compute Cluster subsystem resources (Clusters, ClusterLoadAssignments)
1. Compute Listener subsystem resources (RouteConfigurations, Listeners)
1. Generate an xDS Snapshot
1. Return the xDS Snapshot, ResourceReports and ProxyReport

## Cluster Subsystem Translation

## Listener Subsystem Translation
