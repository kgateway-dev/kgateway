## glooctl install kube

install Gloo on kubernetes

### Synopsis

requires kubectl to be installed

```
glooctl install kube [flags]
```

### Options

```
      --docker-email string       Email for docker registry. Use for pulling private images.
      --docker-password string    Password for docker registry authentication. Use for pulling private images.
      --docker-server string      Docker server to use for pulling images (default "https://index.docker.io/v1/")
      --docker-username string    Username for Docker registry authentication. Use for pulling private images.
  -d, --dry-run                   Dump the raw installation yaml instead of applying it to kubernetes
  -f, --file string               Path to the gloo install manifest
  -h, --help                      help for kube
      --knative-manifest string   Path to the knative install manifest
```

### Options inherited from parent commands

```
  -i, --interactive   use interactive mode
```

### SEE ALSO

* [glooctl install](glooctl_install.md)	 - install gloo on different platforms

