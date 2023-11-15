Param(
    [Parameter(Mandatory = $false,
        ParameterSetName = "GLOO_VERSION")]
    [String]
    $GLOO_VERSION
)

$currentLocation = (Resolve-Path .\).Path

$openssl_version = & { openssl version } 2>&1

if ($openssl_version -is [System.Management.Automation.ErrorRecord]) {
    $openssl_version.Exception.Message
    Write-Output "Openssl is required to install glooctl"
}

if ([string]::IsNullOrEmpty($GLOO_VERSION)) {

    $GLOO_RELEASES = curl -sH"Accept: application/vnd.github.v3+json" https://api.github.com/repos/solo-io/gloo/releases | ConvertFrom-Json

    $GLOO_VERSIONS = New-Object System.Collections.Generic.List[System.Object]
    foreach ($release in $GLOO_RELEASES) {
        if (-Not ($release.tag_name.Contains("-beta") -Or $release.tag_name.Contains("-patch"))) {
            $GLOO_VERSIONS.Add($release.tag_name)
        }
    }
    $GLOO_VERSIONS = $GLOO_VERSIONS | Sort-Object -Descending 
}
else {
    if (-Not $GLOO_VERSION.ToLower().Contains("v")) {
        $GLOO_VERSION = "v" + $GLOO_VERSION
    }
    $GLOO_VERSIONS = $GLOO_VERSION
}

$tmp = "$env:userprofile\AppData\Local\Temp\"
Set-Location $tmp

$glooctlDownloaded = $false
Foreach ($gloo_version IN $GLOO_VERSIONS) {

    $filename = "glooctl-windows-amd64.exe"
    $url = "https://github.com/solo-io/gloo/releases/download/$gloo_version/$filename"
    
    Write-Output "Attempting to download glooctl version $gloo_version"

    $response = curl -f -s $url 
    if ([string]::IsNullOrEmpty($response)) {
        continue
    }

    $glooctlDownloaded = $true

    Write-Output "Downloading $filename..."

    $TMP_SHA = curl -sL ($url + ".sha256")
    $SHA = $TMP_SHA.SubString(0, $TMP_SHA.IndexOf(' '))
    curl -sLO $url
    Write-Output "Download complete!, validating checksum..."

    $checksum = $(openssl dgst -sha256 $filename)

    $checksum = $checksum.Substring($checksum.IndexOf(' '), ($checksum.Length - $checksum.IndexOf(' '))).TrimStart(" ")
    if ($checksum -ne $SHA) {
        Write-Output "Checksum validation failed."
    }

    Write-Output "Checksum valid."

    break;
}

if ($glooctlDownloaded -eq $true) {

    Rename-Item -Path $filename -NewName "glooctl.exe"
    New-Item -ItemType directory -Path "$env:userprofile\.gloo\bin\" -ErrorVariable capturedErrors -ErrorAction SilentlyContinue
    Move-Item "glooctl.exe" "$env:userprofile\.gloo\bin\" -Force
    
    Set-Location (Get-Item $currentLocation).DirectoryName
    
    Write-Output "Gloo Edge was successfully installed!"
    Write-Output `n
    Write-Output "Add the gloo CLI to your path with:"
    Write-Output '  $env:Path += ";$env:userprofile/.gloo/bin/"'
    Write-Output `n
    Write-Output "Now run:"
    Write-Output "  glooctl install gateway     # install gloo's function gateway functionality into the 'gloo-system' namespace"
    Write-Output "  glooctl install ingress     # install very basic Kubernetes Ingress support with Gloo into namespace gloo-system"
    Write-Output "  glooctl install knative     # install Knative serving with Gloo configured as the default cluster ingress"
    Write-Output "Please see visit the Gloo Installation guides for more:  https://docs.solo.io/gloo-edge/latest/installation/"
}
else {
    Set-Location (Get-Item $currentLocation).DirectoryName
    Write-Output "No versions of glooctl found."
}