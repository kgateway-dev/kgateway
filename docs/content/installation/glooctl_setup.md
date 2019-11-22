### Install command line tool (CLI)

The `glooctl` command line provides useful functions to install, configure, and debug Gloo, though it is not required to use Gloo.

* To install `glooctl` using the [Homebrew](https://brew.sh) package manager, run the following.

  ```shell
  brew install solo-io/tap/glooctl
  ```

* To install on any platform run the following.

  ```bash
  curl -sL https://run.solo.io/gloo/install | sh
  export PATH=$HOME/.gloo/bin:$PATH
  ```

* You can download `glooctl` directly via the GitHub releases page. You need to add `glooctl` to your system's `PATH` after downloading.

Verify the CLI is installed and running correctly with:

```bash
glooctl version
```
The command returns your client version and a missing server version (we have not installed Gloo yet!):
```shell
Client: {"version":"1.0.0"}
Server: version undefined, could not find any version of gloo running
```
