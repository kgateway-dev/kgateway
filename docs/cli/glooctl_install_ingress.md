---
title: "glooctl install ingress"
weight: 5
---
## glooctl install ingress

install the Gloo Ingress Controller on kubernetes

### Synopsis

requires kubectl to be installed

```
glooctl install ingress [flags]
```

### Options

```
  -d, --dry-run          Dump the raw installation yaml instead of applying it to kubernetes
  -f, --file string      Install Gloo from this kubernetes manifest yaml file rather than from a release
  -h, --help             help for ingress
      --release string   install using this release version. defaults to the latest github release
```

### Options inherited from parent commands

```
  -i, --interactive   use interactive mode
```

### SEE ALSO

* [glooctl install](glooctl_install.md)	 - install gloo on different platforms

