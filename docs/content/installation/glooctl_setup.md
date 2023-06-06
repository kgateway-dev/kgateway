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

* **Direct download**: You can download `glooctl` directly via the GitHub releases page.
  1. In your browser, navigate to the [Gloo project releases](https://github.com/solo-io/gloo/releases).
  2. Choose the version to upgrade `glooctl` to. For Gloo Edge Enterprise, use the Gloo Edge OSS version that corresponds to the Gloo Edge Enterprise version you want to upgrade to. To find the OSS version that corresponds to each Gloo Edge Enterprise release, see the [Gloo Edge Enterprise changelogs](https://docs.solo.io/gloo-edge/latest/reference/changelog/enterprise/).
  3. Click the version of `glooctl` that you want to install.
  4. In the **Assets**, download the `glooctl` package that matches your operating system, and follow your operating system procedures for replacing your existing `glooctl` binary file with the upgraded version.
  5. After downloading, rename the executable to `glooctl` and add it to your system's `PATH`.

### Update glooctl CLI version {#update-glooctl}

When it's time to upgrade Gloo Edge, make sure to update the `glooctl` version before upgrading.

You can use the `glooctl upgrade` command to upgrade or roll back the `glooctl` version. For example, you might change versions during an upgrade process, or when you have multiple versions of Gloo Edge across clusters that you manage from the same workstation. For more options, run `glooctl upgrade --help`.

1. Set the version to upgrade `glooctl` to in an environment variable. Include the patch version. For Gloo Edge Enterprise, specify the Gloo Edge OSS version that corresponds to the Gloo Edge Enterprise version you want to upgrade to. To find the OSS version that corresponds to each Gloo Edge Enterprise release, see the [Gloo Edge Enterprise changelogs](https://docs.solo.io/gloo-edge/latest/reference/changelog/enterprise/).
   ```sh
   export GLOOCTL_VERSION=<version>
   ```
   
2. Upgrade your version of `glooctl`.
   ```bash
   glooctl upgrade --release v${GLOOCTL_VERSION}
   ```

### Verify the installation or update {#verify-glooctl}

Verify the `glooctl` CLI is installed and running the appropriate version. In the output, the **Client** is your local version. The **Server** is the version that runs in your cluster, and is `undefined` if Gloo Edge is not installed yet.

```bash
glooctl version
```