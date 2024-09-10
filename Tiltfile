# -*- mode: Python -*-

update_settings(k8s_upsert_timeout_secs = 600)
load('ext://helm_resource', 'helm_resource')
load("ext://uibutton", "cmd_button", "location")

kubectl_cmd = "kubectl"
helm_cmd = "helm"
if str(local("command -v " + kubectl_cmd + " || true", quiet = True)) == "":
    fail("Required command '" + kubectl_cmd + "' not found in PATH")

if str(local("command -v " + helm_cmd + " || true", quiet = True)) == "":
    fail("Required command '" + helm_cmd + "' not found in PATH")

settings = {
    "helm_installation_name": "gloo-oss",
    "helm_installation_namespace": "gloo-system",
    "helm_values_file": "./test/kube2e/helm/artifacts/helm.yaml",
}

tilt_file = "./tilt-settings.yaml" if os.path.exists("./tilt-settings.yaml") else "./tilt-settings.json"
settings.update((read_yaml(
    tilt_file,
    default = {},
)))

gloo_installed_cmd = "{0} -n {1} status {2} || true".format(helm_cmd, settings.get("helm_installation_namespace"), settings.get("helm_installation_name"))
gloo_status = str(local(gloo_installed_cmd, quiet = True))
gloo_installed = "STATUS: deployed" in gloo_status

tilt_helper_dockerfile = """
# Tilt image
FROM golang:latest as tilt-helper
# Install delve. Note this should be kept in step with the Go release minor version.
RUN go install github.com/go-delve/delve/cmd/dlv@latest
# Support live reloading with Tilt
RUN wget --output-document /restart.sh --quiet https://raw.githubusercontent.com/tilt-dev/rerun-process-wrapper/master/restart.sh  && \
    wget --output-document /start.sh --quiet https://raw.githubusercontent.com/tilt-dev/rerun-process-wrapper/master/start.sh && \
    chmod +x /start.sh && chmod +x /restart.sh && chmod +x /go/bin/dlv && \
    touch /process.txt && chmod 0777 /process.txt `# pre-create PID file to allow even non-root users to run the image`
"""

tilt_dockerfile = """
FROM golang:latest as tilt
WORKDIR /app
COPY --from=tilt-helper /go/bin/dlv /go/bin/dlv
COPY --from=tilt-helper /process.txt .
COPY --from=tilt-helper /start.sh .
COPY --from=tilt-helper /restart.sh .
COPY --from=tilt-helper /go/bin/dlv .
COPY $binary_name .
RUN chmod 777 ./$binary_name
"""

standard_entrypoint = "ENTRYPOINT /app/start.sh /app/$binary_name"
debug_entrypoint = "ENTRYPOINT /app/start.sh /go/bin/dlv --listen=0.0.0.0:$debug_port --api-version=2 --headless=true --only-same-user=false --accept-multiclient --check-go-version=false exec /app/$binary_name"

get_resources_cmd = "{0} -n {1} template {2} --include-crds install/helm/gloo/ --set license_key='abcd' --values={3}".format(helm_cmd, settings.get("helm_installation_namespace"), settings.get("helm_installation_name"), settings.get("helm_values_file"))

arch = str(local("make print-GOARCH", quiet = True)).strip()

def get_deployment(resources, name) :
    for resource in resources:
        if resource["kind"] == "Deployment":
            if resource["metadata"]["name"] == name :
                return resource

def get_resources() :
    return decode_yaml_stream(str(local(get_resources_cmd, quiet = True)))

resources = get_resources()

def build_go_binary(provider):
    live_reload_deps = provider.get("live_reload_deps", [])
    if provider.get("build_binary") :
        # Build the go binary
        # Ref: https://docs.tilt.dev/api.html#api.local_resource
        # resource_deps = []
        # if not gloo_installed :
        #     resource_deps = [settings.get("helm_installation_name")]
        local_resource(
            provider.get("label") + "_binary",
            cmd = provider.get("build_binary"),
            deps = live_reload_deps,
            labels = [provider.get("label"), "binaries"],
            allow_parallel = True,
            # resource_deps = resource_deps,
        )

def build_docker_image(provider):
    if not provider.get("live_reload_deps") :
        return
    if provider.get("dockerfile_contents") :
        dockerfile_contents = tilt_helper_dockerfile + "\n" + provider.get("dockerfile_contents")
    else :
        dockerfile_contents = "\n".join([
            tilt_helper_dockerfile,
            tilt_dockerfile,
        ])
        if provider.get("debug_port") :
            dockerfile_contents = dockerfile_contents + debug_entrypoint
        else :
            dockerfile_contents = dockerfile_contents + standard_entrypoint

    dockerfile_contents = dockerfile_contents.replace("$binary_name", provider.get("binary_name"))
    dockerfile_contents = dockerfile_contents.replace("$debug_port", str(provider.get("debug_port")))

    binary_path = provider.get("binary_path", "/app/" + provider.get("binary_name"))

    # Build the image and sync it on binary file changes
    # Ref: https://docs.tilt.dev/api.html#api.local_resource
    docker_build(
        ref = provider.get("image"), # name of the image
        context = provider.get("context"),
        dockerfile_contents = dockerfile_contents,
        build_args = {"binary_name": provider.get("binary_name")},
        target = "tilt", # The final build stage. Any custom dockerfile must have the final stage as `FROM xxx AS tilt``
        only = provider.get("binary_name"), # Rebuild only if this file changes
        live_update = [
            # Copy over the binary to the container
            sync(provider.get("context") + "/" + provider.get("binary_name"), binary_path),
            # Restart the main script
            run("cd /app;  ./restart.sh"),
        ],
    )

def get_port_forwards(provider):
    # Ensure the port forwards to the corresponding port in the container
    port_forwards = []
    for pf in provider.get("port_forwards", []) :
        if type(pf) == "int" :
            port_forwards.append("{0}:{0}".format(pf))
    # Ensure the debug port is accessible
    debug_port = provider.get("debug_port")
    if debug_port:
        if debug_port not in provider.get("port_forwards", []) :
            port_forwards.append("{0}:{0}".format(debug_port))
    return port_forwards

def get_links(provider):
    links = []
    for l in provider.get("links", []) :
        links.append(link(l, l.rpartition('/')[-1]))
    return links

def enable_provider(provider):
    label = provider.get("label").lower()
    provider["port_forwards"] = get_port_forwards(provider)
    provider["links"] = get_links(provider)
    provider["binary_name"] = provider.get("binary_name").replace("$ARCH", arch)

    build_go_binary(provider)
    build_docker_image(
        provider,
    )

    deployment = get_deployment(resources, label)
    if provider.get("live_reload_deps") :
        # Overwrite the deployment image name with our custom one
        deployment["spec"]["template"]["spec"]["containers"][0]["image"] = provider.get("image")

    # We need to run as root to avoid pesky permission issues when copying the new binary over to the running container and restarting it
    if deployment["spec"]["template"]["spec"]["containers"][0].pop("securityContext", None) :
        deployment["spec"]["template"]["spec"]["containers"][0]["securityContext"] = {"runAsNonRoot": False, "runAsUser": 0, "readOnlyRootFilesystem": False}
    if deployment["spec"]["template"]["spec"].pop("securityContext", None) :
        deployment["spec"]["template"]["spec"]["securityContext"] = {"runAsNonRoot": False, "runAsUser": 0, "readOnlyRootFilesystem": False}

    # Apply the deployment and let tilt manage it
    # Ref: https://docs.tilt.dev/api.html#api.k8s_yaml
    k8s_yaml(encode_yaml(deployment), allow_duplicates = True)

    resource_deps = []
    if provider.get("live_reload_deps") :
        resource_deps = [label + "_binary"]
    if not gloo_installed :
        resource_deps = [settings.get("helm_installation_name")]

    # Create and manage the tweaked deployment
    # Ref: https://docs.tilt.dev/api.html#api.k8s_resource
    k8s_resource(
        workload = label,
        port_forwards = provider.get("port_forwards"),
        links = provider.get("links"),
        new_name = label.lower() + "_controller",
        labels = [label, "controllers"],
        resource_deps = resource_deps,
    )

def enable_providers():
    for provider in settings["enabled_providers"] :
        enable_provider(settings["providers"][provider])

def install_gloo():
    if not gloo_installed :
        install_helm_cmd = """
            kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.0.0/standard-install.yaml ;
            {0} upgrade --install -n {1} --create-namespace {2} install/helm/gloo/ --set license_key='$GLOO_LICENSE_KEY' --set gloo.deployment.glooContainerSecurityContext.readOnlyRootFilesystem=false --values={3}""".format(helm_cmd, settings.get("helm_installation_namespace"), settings.get("helm_installation_name"), settings.get("helm_values_file"))

        local_resource(
            name = settings.get("helm_installation_name"),
            cmd = ["bash", "-c", install_helm_cmd],
            auto_init = True,
            trigger_mode = TRIGGER_MODE_MANUAL,
            labels = [settings.get("helm_installation_name")],
        )

        cmd_button(
            name="install-edge",
            text="Install / Upgrade Edge",
            resource=settings.get("helm_installation_name"),
            # location=location.NAV,
            argv = ["sh", "-c", install_helm_cmd],
            icon_name='deployed_code')
        enable_providers()


def validate_registry() :
    usingLocalRegistry = str(local(kubectl_cmd + " get cm -n kube-public local-registry-hosting || true", quiet = True))
    if not usingLocalRegistry:
        fail("default_registry is required when not using a local registry. Try running ./kind-install-for-gloo.sh")

validate_registry()
install_gloo()
if gloo_installed :
    enable_providers()
