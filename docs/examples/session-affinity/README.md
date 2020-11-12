# Session Affinity Demo App

This is a very simple app for demonstrating Gloo Edge's session affinity (sticky sessions) capabilities.

It can be deployed in a `DaemonSet` in a multi-node Kubernetes cluster to test session affinity configuration.

The app simply returns the number of times it was queried.
