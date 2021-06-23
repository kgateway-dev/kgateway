---
title: External Auth Custom Cert Authority
weight: 80
description: Configuring a custom certificate authority for extauth to use.
---

Gloo Edge Enterprise includes external authentication, which allows you to offload authentication responsibilities from Envoy to an external authentication server. There may be cases where you need the external authentication server to trust certificates issued from a custom certificate authority. In this guide, we will show you how to add the certificate authority during Gloo Edge Enterprise installation or after installation is complete.

The external authentication server runs as its own Kubernetes pod or as a sidecar to the `gateway-proxy` pods. The certificate authority public certificate will be saved as Kubernetes secret, and then an initialization container will be used to inject the CA certificate into the list of trusted certificate authorities for the external authentication pods. 

For this guide, we will create a temporary certificate authority using OpenSSL. In a production scenario, you would retrieve the public certificate from an existing certificate authority you wish to be trusted.

This guide assumes that you already have a Kubernetes cluster available for installation of Gloo Edge Enterprise, or that you have a running instance of Gloo Edge Enterprise.

## Create a certificate authority

We are going to use OpenSSL to create a simple certificate authority and upload the public certificate as a Kubernetes secret. First let's create the certificate authority:

```bash
# Enter whatever passphrase you'd like
openssl genrsa -des3 -out ca.key 4096

# Enter glooe.example.com for the Common Name, leave all other defaults
openssl req -new -x509 -days 365 -key ca.key -out ca.cert.pem
```

Now we are a certificate authority! Let's go ahead and get the `ca.cert.pem` file added as a Kubernetes secret in our cluster.

```bash
# Create the gloo-system namepace if it doesn't exist
kubectl create namespace gloo-system

# Add the CA pem file as a generic secret
kubectl create secret generic trusted-ca --from-file=tls.crt=ca.cert.pem -n gloo-system
```

Now we are ready to either [install Gloo Edge Enterprise](#install-gloo-edge-enterprise) or [update an existing Gloo Edge Enterprise installation](#update-gloo-edge-enterprise).

## Install Gloo Edge Enterprise

To add the customization of a trusted certificate authority to the Gloo Edge Enterprise installation, we are going to need to use Helm for the installation and `kustomize` to perform a last mile helm chart customization, as outlined [in this guide]({{< versioned_link_path fromRoot="/installation/gateway/kubernetes/helm_advanced/" >}}). Essentially, we are going to have Helm render an installation manifest, and then tailor the manifest using kustomize to add the necessary configuration.

```bash
# Add the Gloo Edge Enterprise repo to Helm if you haven't already
helm repo add glooe https://storage.googleapis.com/gloo-ee-helm
helm repo update

# Grab the current Gloo Edge Enterprise version
version=$(helm search repo glooe -ojson | jq .[0].version -r)

# Create the patch
cat > custom-ca-patch.yaml <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: extauth
spec:
  template:
    spec:
      volumes:
      - name: certs
        emptyDir: {}
      - name: ca-certs
        secret:
          secretName: trusted-ca
          items:
          - key: tls.crt
            path: ca.crt
      initContainers:
      - name: add-ca-cert
        image: quay.io/solo-io/extauth-ee:$version
        command:
          - sh
        args:
          - "-c"
          - "cp -r /etc/ssl/certs/* /certs; cat /etc/ssl/certs/ca-certificates.crt /ca-certs/ca.crt > /certs/ca-certificates.crt"
        volumeMounts:
          - name: certs
            mountPath: /certs
          - name: ca-certs
            mountPath: /ca-certs
EOF
```

Now we'll create a helper script to let kustomize do the post-render work from Helm:

```bash
cat > kustomize.sh <<EOF
#!/bin/sh
cat > base.yaml
# you can also use "kustomize build ." if you have it installed.
exec kubectl kustomize
EOF
chmod +x ./kustomize.sh
```

And create the `kustomization.yaml` that includes our patch:

```bash
cat > kustomization.yaml <<EOF
resources:
- base.yaml
patchesStrategicMerge:
- custom-ca-patch.yaml
EOF
```

Finally, we'll install Gloo Edge Enterprise with Helm using kustomize to add our patch in. Be sure to update the value for the license key.

```bash
helm install gloo glooe/gloo-ee --namespace gloo-system \
  --set-string license_key=LICENSE_KEY \
  --post-renderer ./kustomize.sh
```

Once the installation is complete, we can validate our change with the following command:

```bash
kubectl describe pods -n gloo-system -l gloo=extauth
```

You should see the init container `add-ca-cert` has completed its work.

```bash
  State:          Terminated
    Reason:       Completed
    Exit Code:    0
```

You've successfully added a custom certificate authority for external authentication!

## Update Gloo Edge Enterprise

To update an existing Gloo Edge Enterprise installation to support an additional trusted root certificate authority, we are going to patch the deployment for the external authentication server. You can do this by using `kubectl patch`. We are going to add three values for the volume, volumeMount, and initialization container.

```bash
# Get the current image of the extauth pod
image=$(kubectl get deploy/extauth -n gloo-system -ojson | jq .spec.template.spec.containers[0].image -r)

# Patch the deployment with an initialization pod
cat  <<EOF | xargs -0 kubectl patch deployment -n gloo-system extauth --type='json' -p
[
    {
        "op": "add",
        "path": "/spec/template/spec/containers/0/volumeMounts",
        "value": [
            {
                "name": "certs",
                "mountPath": "/etc/ssl/certs/"
            }
        ]
    },
    {
        "op": "add",
        "path": "/spec/template/spec/volumes",
        "value": [
            {
                "name": "certs",
                "emptyDir": {}
            },
            {
                "name": "ca-certs",
                "secret": {
                    "secretName": "trusted-ca",
                    "items": [
                        {
                            "key": "tls.crt",
                            "path": "ca.crt"
                        }
                    ]
                }
            }
        ]
    },
    {
        "op": "add",
        "path": "/spec/template/spec/initContainers",
        "value": [
            {
                "name": "add-ca-cert",
                "image": "$image",
                "command": [
                    "sh"
                ],
                "args": [
                    "-c",
                    "cp -r /etc/ssl/certs/* /certs; cat /etc/ssl/certs/ca-certificates.crt /ca-certs/ca.crt > /certs/ca-certificates.crt"
                ],
                "volumeMounts": [
                    {
                        "name": "certs",
                        "mountPath": "/certs"
                    },
                    {
                        "name": "ca-certs",
                        "mountPath": "/ca-certs"
                    }
                ]
            }
        ]
    }
]
EOF

```

This will force a recreation of the external authentication server pod(s). We can validate our trusted certificate authority was added by running the following:

```bash
kubectl describe pods -n gloo-system -l gloo=extauth
```

You should see the init container `add-ca-cert` has completed its work.

```bash
  State:          Terminated
    Reason:       Completed
    Exit Code:    0
```

You've successfully added a custom certificate authority for external authentication!

## Summary

In this guide you learned how to add a custom certificate authority as trusted to the external authentication server. If you want to know more about the capabilities of external authentication, be sure to check out our [guides]({{< versioned_link_path fromRoot="/guides/security/auth/extauth/" >}}).