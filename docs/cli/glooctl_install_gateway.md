## glooctl install gateway

install the Gloo Gateway on kubernetes

### Synopsis

requires kubectl to be installed

```
glooctl install gateway [flags]
```

### Options

```
  -d, --dry-run          Dump the raw installation yaml instead of applying it to kubernetes
  -f, --file string      Install Gloo from this kubernetes manifest yaml file rather than from a release
  -h, --help             help for gateway
      --release string   install using this release version. defaults to the latest github release
```

### Options inherited from parent commands

```
  -i, --interactive   use interactive mode
```

### SEE ALSO

* [glooctl install](glooctl_install.md)	 - install gloo on different platforms

