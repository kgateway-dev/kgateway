# Installing Gloo Edge

## Installing on a Local Filesystem
Refer to our [external documentation](https://docs.solo.io/gloo-edge/latest/installation/gateway/development/docker-compose-file/)

## Installing on a Consul
Refer to our [external documentation](https://docs.solo.io/gloo-edge/latest/installation/gateway/development/docker-compose-consul/)

# Installing on Kubernetes

Note: if running on GKE, you need to configure permission to create rbac: 
```bash
kubectl create clusterrolebinding --user <gcloud-email> <crb-name> --clusterrole=<any role with RBAC create permission>
```

# Installing on Nomad
> Note: These steps may not work as they have not been updated recently

Steps for creating a local Nomad deployment from scratch (assuming you have `nomad`, `consul`, and `vault` binaries installed) lives in the `nomad/` directory.
