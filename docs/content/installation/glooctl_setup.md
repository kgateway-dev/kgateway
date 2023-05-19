### Install the Gloo Edge command line tool (CLI) {#install-glooctl}

You can install the Gloo Edge command line, `glooctl`, to help install, configure, and debug Gloo Edge. Depending on your operating system, you have several installation options.

* **macOS**: You can use the [Homebrew](https://brew.sh) package manager.

  ```shell
  brew install glooctl
  ```

* **Most platforms**: You can use the following installation script, which requires Python to execute properly.

  ```bash
  curl -sL https://run.solo.io/gloo/install | sh
  export PATH=$HOME/.gloo/bin:$PATH
  ```

* **Windows**: You can use the following installation script, which requires OpenSSL to execute properly.
  
  ```pwsh
  (New-Object System.Net.WebClient).DownloadString("https://run.solo.io/gloo/windows/install") | iex
  $env:Path += ";$env:userprofile/.gloo/bin/"
  ```

* **Direct download**: You can download `glooctl` directly via the [GitHub releases page](https://github.com/solo-io/gloo/releases). After downloading, rename the executable to `glooctl` and add it to your system's `PATH`.

### Update glooctl CLI version {#update-glooctl}

If you already installed `glooctl`, make sure to update `glooctl` to the same minor version as the version of Gloo Edge that is installed in your cluster. For example, if you're using Gloo Edge 1.13.8, you should use a 1.13.8 release of `glooctl`.

You can use the `glooctl upgrade` command to set the `--release` that you want to use. You can use this command to upgrade or roll back the `glooctl` version. For example, you might change versions during an upgrade process, or when you have multiple versions of Gloo Edge across clusters that you manage from the same workstation.

```bash
glooctl upgrade --release v1.13.8
```

### Verify the installation or update {#verify-glooctl}

You can verify the `glooctl` CLI is installed and running the appropriate version.

```bash
glooctl version
```

In the output, the **Client** is your local version. The **Server** is the version that runs in your cluster, and is `undefined` if you did not install Gloo Edge yet.

```shell
Client: {"version":"1.13.8"}
Server: version undefined, could not find any version of gloo running
```