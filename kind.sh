NAME=${1:-kind}

kind create cluster --name $NAME

SCRIPTPATH=$( cd "$(dirname "$0")" && pwd -P )

kubectl apply -f $SCRIPTPATH/crds/gateway-crds.yaml

kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.13.7/config/manifests/metallb-native.yaml

# Wait for MetalLB to become available.
kubectl rollout status -n metallb-system deployment/controller --timeout 2m
kubectl rollout status -n metallb-system daemonset/speaker --timeout 2m
kubectl wait -n metallb-system  pod -l app=metallb --for=condition=Ready --timeout=10s

SUBNET=$(docker network inspect  kind -f '{{(index .IPAM.Config 0).Subnet}}'| cut -d '.' -f1,2)
MIN=${SUBNET}.255.0
MAX=${SUBNET}.255.231

kubectl apply -f - <<EOF
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: address-pool
  namespace: metallb-system
spec:
  addresses:
  - ${MIN}-${MAX}
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: advertisement
  namespace: metallb-system
spec:
  ipAddressPools:
  - address-pool
EOF