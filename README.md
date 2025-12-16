# provider-slicervm

`provider-slicervm` is a [Crossplane](https://crossplane.io/) Provider for managing
[Slicer](https://slicervm.com/) VMs. It allows you to create, manage, and delete
Slicer VMs declaratively using Kubernetes custom resources.

## Features

- **VM Management**: Create and delete Slicer VMs with configurable CPU, RAM, and storage
- **Userdata Support**: Configure VMs with cloud-init userdata scripts
- **SSH Key Management**: Add SSH keys directly or import from GitHub users
- **Tag Support**: Apply tags to VMs for organization and filtering
- **Connection Details**: Automatically publish VM hostname and IP to Kubernetes secrets

## Installation

### Prerequisites

- Kubernetes cluster with Crossplane installed
- Slicer API endpoint and authentication token

### Install the Provider

```bash
# Apply the CRDs
kubectl apply -f package/crds/

# Deploy the provider (adjust for your deployment method)
kubectl apply -f package/
```

## Configuration

### Create a Secret with Slicer Credentials

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: slicer-credentials
  namespace: default
type: Opaque
stringData:
  token: "your-slicer-api-token"
```

### Create a ProviderConfig

```yaml
apiVersion: template.crossplane.io/v1alpha1
kind: ClusterProviderConfig
metadata:
  name: default
spec:
  url: "http://127.0.0.1:8080"  # Slicer API endpoint
  hostGroup: "api"              # Default host group
  credentials:
    source: Secret
    secretRef:
      namespace: default
      name: slicer-credentials
      key: token
```

## Usage

### Create a VM

```yaml
apiVersion: vm.slicervm.crossplane.io/v1alpha1
kind: VM
metadata:
  name: my-vm
spec:
  forProvider:
    cpus: 2
    ramGb: 4
    hostGroup: "api"
    importUser: "github-username"  # Import SSH keys from GitHub
    tags:
      - crossplane
      - production
  providerConfigRef:
    kind: ClusterProviderConfig
    name: default
  writeConnectionSecretToRef:
    name: my-vm-connection
```

### VM Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `hostGroup` | string | from ProviderConfig | Host group to create the VM in |
| `cpus` | int | 2 | Number of virtual CPUs |
| `ramGb` | int | 4 | Amount of RAM in GB |
| `userdata` | string | - | Cloud-init userdata script |
| `sshKeys` | []string | - | List of SSH public keys |
| `importUser` | string | - | GitHub username to import SSH keys from |
| `tags` | []string | - | Tags to apply to the VM |

### Check VM Status

```bash
kubectl get vms.vm.slicervm.crossplane.io

NAME     READY   SYNCED   EXTERNAL-NAME   HOSTNAME   IP               AGE
my-vm    True    True     api-1           api-1      192.168.137.2    5m
```

### Connection Secret

The VM's connection details (hostname and IP) are published to the secret specified in `writeConnectionSecretToRef`:

```bash
kubectl get secret my-vm-connection -o yaml
```

## Development

### Building

```bash
# Generate code and CRDs
go generate ./apis/...

# Build the provider
go build ./...
```

### Running Locally

```bash
# Run the provider out-of-cluster
go run cmd/provider/main.go --debug
```

## License

Apache 2.0 - See [LICENSE](LICENSE) for more information.
