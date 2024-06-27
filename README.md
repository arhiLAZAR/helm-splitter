# Overview
This tool generates templates from a Helm chart, splits multi-manifest templates into separate files, and adds resource-type prefixes to these files. After that, you can commit the files to Git and use Flux, ArgoCD, or similar tools to deploy them to Kubernetes without the need for Helm operators or controllers.

# Pre-installation requirements
[Install Helm](https://helm.sh/docs/intro/install/). The tool uses helm commands to generate templates.

# How to install
1) Download a binary from [Releases](https://github.com/arhiLAZAR/helm-splitter/releases) or build it yourself (see below).
2) Copy it to `/usr/local/bin/` or another bin directory.

# Example
```bash
helm-splitter --chart thanos \
--version 15.7.9 \
--repository https://charts.bitnami.com/bitnami \
--custom-values-file values.yaml \
--namespace monitoring
```
Change variables for your desired helm.

# Parameters
| Flag | Description | Default | Mandatory? |
| ------------- | ------------- | ------------- | ------------- |
| --chart | Name of the helm chart | - | yes |
| --version | Version of the helm chart | \<latest\> | no |
| --repository | Helm repository | - | yes |
| --custom-values-file | File name with custom helm values.yaml | - | no |
| --namespace | Kubernetes namespace the chart will be installed in | - | yes |
| --skip-crds | By default, the tool generates CRDs. Use the flag to skip this step | false | no |
| --overwrite | Allow the tool to overwrite existing output files | false | no |
| --config | Path to the config file (see below) | - | no |
| --debug | Enable debug output | false | no |

# Config
Config file is used to store shortcuts for different kubernetes kinds.
## Structure
```yaml
shortcuts:
    Kind1: shortcut1
    Kind2: shortcut2
```
## Example
```yaml
shortcuts:
    ClusterRoleBinding: crb
    ConfigMap: cm
    CustomResourceDefinition: crd
    Deployment: dep
    Service: svc
    ServiceAccount: sa
```
## Available config paths
1) If `--config </path/to/config>` is provided, the tool uses this file.
2) If not - the tool checks if `~/.helm-splitter.yaml` is present.
3) If not - the tool checks if `/etc/helm-splitter/config.yaml` is present.
4) If the tool still cannot find the config, it creates a new file `~/.helm-splitter.yaml` with default shortcuts and uses it.

# How to build
```bash
go build -o helm-splitter cmd/*.go
```
