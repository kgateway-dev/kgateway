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
