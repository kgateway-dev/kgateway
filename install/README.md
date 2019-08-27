# Installing on Kubernetes

Note: if running on GKE, you need to configure permission to create rbac: 
```bash
kubectl create clusterrolebinding --user <gcloud-email> <crb-name> --clusterrole=<any role with RBAC create permission>
```

# Installing on Nomad

Steps for creating a local Nomad deployment from scratch (assuming you have `nomad`, `consul`, and `vault` binaries installed) lives in the `nomad/` directory.
