---
title: "glooctl completion"
weight: 5
---
## glooctl completion

generate auto completion for your shell

### Synopsis


	Output shell completion code for the specified shell (bash or zsh).
	The shell code must be evaluated to provide interactive
	completion of glooctl commands.  This can be done by sourcing it from
	the .bash_profile.
	Note for zsh users: [1] zsh completions are only supported in versions of zsh >= 5.2

```
glooctl completion SHELL [flags]
```

### Examples

```

	# Installing bash completion on macOS using homebrew
	## If running Bash 3.2 included with macOS
	  	brew install bash-completion
	## or, if running Bash 4.1+
	    brew install bash-completion@2
	## You may need add the completion to your completion directory
	    glooctl completion bash > $(brew --prefix)/etc/bash_completion.d/glooctl
	# Installing bash completion on Linux
	## Load the glooctl completion code for bash into the current shell
	    source <(glooctl completion bash)
	## Write bash completion code to a file and source if from .bash_profile
	    glooctl completion bash > ~/.glooctl/completion.bash.inc
	    printf "
 	     # glooctl shell completion
	      source '$HOME/.glooctl/completion.bash.inc'
	      " >> $HOME/.bash_profile
	    source $HOME/.bash_profile
	# Load the glooctl completion code for zsh[1] into the current shell
	    source <(glooctl completion zsh)
	# Set the glooctl completion code for zsh[1] to autoload on startup
	    glooctl completion zsh > "${fpath[1]}/_glooctl"
```

### Options

```
  -h, --help   help for completion
```

### Options inherited from parent commands

```
  -c, --config string              set the path to the glooctl config file (default "<home_directory>/.gloo/glooctl-config.yaml")
      --consul-address string      address of the Consul server. Use with --use-consul (default "127.0.0.1:8500")
      --consul-allow-stale-reads   Allows reading using Consul's stale consistency mode.
      --consul-datacenter string   Datacenter to use. If not provided, the default agent datacenter is used. Use with --use-consul
      --consul-root-key string     key prefix for for Consul key-value storage. (default "gloo")
      --consul-scheme string       URI scheme for the Consul server. Use with --use-consul (default "http")
      --consul-token string        Token is used to provide a per-request ACL token which overrides the agent's default token. Use with --use-consul
  -i, --interactive                use interactive mode
      --kube-context string        kube context to use when interacting with kubernetes
      --kubeconfig string          kubeconfig to use, if not standard one
      --use-consul                 use Consul Key-Value storage as the backend for reading and writing config (VirtualServices, Upstreams, and Proxies)
```

### SEE ALSO

* [glooctl](../glooctl)	 - CLI for Gloo

