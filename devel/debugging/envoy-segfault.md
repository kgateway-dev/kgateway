# Envoy Segmentation Fault

When a segmentation fault occurs in the gateway-proxy pod, the stack trace is not visible. If you are running Gloo Edge Enterprise, replace the default docker image in that pod, with the debug replica:

Original Image:
```bash
$(IMAGE_REGISTRY)/gloo-ee-envoy-wrapper:$(VERSION)
```

Updated Image:
```bash
$(IMAGE_REGISTRY)/gloo-ee-envoy-wrapper:$(VERSION)-debug
```

This new image should emit more information about the segmentation fault, that you can include in a GitHub issue tracking the behavior.