ISTIO_VERSION="${ISTIO_VERSION:-1.22.0}"

TARGET_ARCH=x86_64
if [[ $ARCH == 'arm64' ]]; then
  TARGET_ARCH=arm64
fi

curl -L https://istio.io/downloadIstio | ISTIO_VERSION=$ISTIO_VERSION TARGET_ARCH=$TARGET_ARCH sh -
yes | "./istio-$ISTIO_VERSION/bin/istioctl" install --set profile=minimal

# Setup bookinfo to test istio integration
kubectl create namespace bookinfo

kubectl label namespace bookinfo istio-injection=enabled

kubectl apply -f https://raw.githubusercontent.com/istio/istio/release-1.20/samples/bookinfo/platform/kube/bookinfo.yaml -n bookinfo
kubectl rollout status deployment productpage-v2 -n bookinfo
kubectl rollout status deployment details-v1 -n bookinfo
kubectl rollout status deployment reviews-v1 -n bookinfo
kubectl rollout status deployment reviews-v2 -n bookinfo
kubectl rollout status deployment reviews-v3 -n bookinfo
kubectl rollout status deployment ratings-v1 -n bookinfo