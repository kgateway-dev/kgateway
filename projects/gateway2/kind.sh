kind create cluster

kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.0.0-rc1/standard-install.yaml

kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.13.7/config/manifests/metallb-native.yaml

# Wait for MetalLB to become available.
kubectl rollout status -n metallb-system deployment/controller --timeout 5m
kubectl rollout status -n metallb-system daemonset/speaker --timeout 5m
kubectl wait --namespace metallb-system \
                --for=condition=ready pod \
                --selector=app=metallb \
                --timeout=90s

# Apply config with addresses based on docker network IPAM.
subnet=$(docker network inspect kind | jq -r '.[].IPAM.Config[].Subnet | select(contains(":") | not)')
# Assume default kind network subnet prefix of 16, and choose addresses in that range.
address_first_octets=$(echo "${subnet}" | awk -F. '{printf "%s.%s",$1,$2}')
address_range="${address_first_octets}.255.200-${address_first_octets}.255.250"
kubectl apply -f - <<EOF
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  namespace: metallb-system
  name: kube-services
spec:
  addresses:
  - ${address_range}
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: kube-services
  namespace: metallb-system
spec:
  ipAddressPools:
  - kube-services
EOF
