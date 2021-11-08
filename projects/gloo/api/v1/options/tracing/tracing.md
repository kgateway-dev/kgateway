# Setting up zipkin tracing locally

After following the guide [here](https://docs.solo.io/gloo-edge/latest/guides/observability/tracing/), use the following command to start the zipkin container

`docker run --network=kind -itd --name zipkin -p 9411:9411 openzipkin/zipkin`

Run `docker network inspect kind` to ensure both zipkin and zipkin-tracing-control-plane are in the kind network.
```json
[
    {
        "Name": "kind",
        "Id": "6a37a4ebb2d0e7dcbabe50dc8b1a519b431f054aebb822ed85e00abde99fd4d3",
        "Created": "2021-09-16T09:28:49.88165506-04:00",
        "Scope": "local",
        "Driver": "bridge",
        "EnableIPv6": true,
        "IPAM": {
            "Driver": "default",
            "Options": {},
            "Config": [
                {
                    "Subnet": "172.18.0.0/16",
                    "Gateway": "172.18.0.1"
                },
                {
                    "Subnet": "fc00:f853:ccd:e793::/64",
                    "Gateway": "fc00:f853:ccd:e793::1"
                }
            ]
        },
        "Internal": false,
        "Attachable": false,
        "Ingress": false,
        "ConfigFrom": {
            "Network": ""
        },
        "ConfigOnly": false,
        "Containers": {
            "3431770d0c41bfbc8eceac4c806605286f5dac81820599f828dcb250037a2f48": {
                "Name": "zipkin-tracing-control-plane",
                "EndpointID": "3e48e18bc7b259ca9d597a594ee3d5205c8339e8ecd9f8f274a178d07f395b78",
                "MacAddress": "02:42:ac:12:00:03",
                "IPv4Address": "172.18.0.3/16",
                "IPv6Address": "fc00:f853:ccd:e793::3/64"
            },
            "84dadbd86f113c7104eca23d3d78e9dec997a47666c1ba4eed2ae7a5ad8eb20d": {
                "Name": "zipkin",
                "EndpointID": "09e07c8ac6b1cd912c325962586d9497520e216a4ec357384c663594248fc104",
                "MacAddress": "02:42:ac:12:00:02",
                "IPv4Address": "172.18.0.2/16",
                "IPv6Address": "fc00:f853:ccd:e793::2/64"
            }
        },
        "Options": {
            "com.docker.network.bridge.enable_ip_masquerade": "true",
            "com.docker.network.driver.mtu": "1500"
        },
        "Labels": {}
    }
]
```

`172.18.0.2` would be the IP to specify as the address for your zipkin cluster [here](https://docs.solo.io/gloo-edge/latest/guides/observability/tracing/#1-configure-a-tracing-cluster)