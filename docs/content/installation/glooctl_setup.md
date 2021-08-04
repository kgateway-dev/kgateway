### Install command line tool (CLI)

The `glooctl` command line provides useful functions to install, configure, and debug Gloo Edge, though it is not required to use Gloo Edge.

* To install `glooctl` using the [Homebrew](https://brew.sh) package manager, run the following.

  ```shell
  brew install glooctl
  ```

* To install on most platforms you can use the install script. Python is required for installation to execute properly.

  ```bash
  curl -sL https://run.solo.io/gloo/install | sh
  export PATH=$HOME/.gloo/bin:$PATH
  ```

* To install on windows you can use this install script. Openssl is required for installation to execute properly.
  
  ```pwsh
  (New-Object System.Net.WebClient).DownloadString("https://run.solo.io/gloo/windows/install") | iex
  $env:Path += ";$env:userprofile/.gloo/bin/"
  ```

* You can download `glooctl` directly via the [GitHub releases page](https://github.com/solo-io/gloo/releases). You will need to rename the executable to `glooctl` and add it to your system's `PATH` after downloading.

You can verify the `glooctl` CLI is installed and running correctly by executing the command:

```bash
glooctl version
```
The command returns your client version and a missing server version (we have not installed Gloo Edge yet!):
```shell
Client: {"version":"1.2.3"}
Server: version undefined, could not find any version of gloo running
```

#### Update glooctl CLI version

You should always try to use the same minor `glooctl` version as the version of Gloo Edge installed in your cluster, i.e., if you're using Gloo Edge 1.6.x, you should use a 1.6.x release of `glooctl`.

Fortunately, `glooctl` is able to update itself to different versions. To change the version of glooctl you currently have installed, you can run:

```bash
glooctl upgrade --release v1.6.0
```

**Note**: The glooctl upgrade command can also be used to roll back your glooctl version to previous releases. This can be convenient if you are using an older version of Gloo Edge and want to use the same verison of glooctl to ensure compatability.
