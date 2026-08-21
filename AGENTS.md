# Agent Development Guide

This file contains instructions and conventions for AI agents (and developers) working on the Ironbark project.

## Project Overview

**Name:** Ironbark
**Language:** Go 1.25.5+
**Purpose:** Kubernetes management tooling that implements the brightSPARK Labs opinionated deployment pattern using Zarf and ArgoCD for air-gapped/disconnected environments.

**Key Technologies:**
- Go (CLI application using Cobra)
- Zarf (air-gap deployment packaging)
- ArgoCD (GitOps continuous deployment)
- Kubernetes (target platform)
- Helm (application packaging)
- Gitea (internal Git server via Zarf)

**Compatibility:**
- Ironbark 0.x.y targets Zarf 0.64.0
- Uses Kubernetes client-go v0.34.3

## Project Structure

```
ironbark/
├── src/                          # Go source code
│   ├── main.go                   # Application entry point
│   ├── go.mod                    # Go module definition
│   ├── cmd/                      # CLI commands (Cobra)
│   │   ├── root.go               # Root command and global flags
│   │   ├── init.go               # Initialisation commands
│   │   └── argo.go               # ArgoCD repository management commands
│   ├── internal/                 # Internal packages
│   │   ├── constants/            # Application constants and env vars
│   │   ├── git/                  # Git operations (Zarf Gitea integration)
│   │   └── zarf/                 # Zarf cluster and package operations
│   └── resources/                # Embedded resources
│       └── resources/
│           ├── bootstrap-argocd-app-of-apps.yaml
│           └── repos/
│               └── ironbark-argocd-app-of-apps/  # App of Apps template
├── resources/                    # External resources
│   └── packages/                 # Zarf package definitions
│       ├── deploy/               # Deployment packages
│       │   └── bsl-ironbark/     # Core infrastructure package
│       └── mirror/               # Mirror packages (images/charts)
│           └── ironbark-canary-app/  # Example application
├── build/                        # Build artifacts (gitignored)
│   ├── bin/                      # Compiled binaries
│   └── images/                   # OCI images
├── Dockerfile                    # Multi-stage build
├── Makefile                      # Build automation
└── README.adoc                   # User documentation

```

## Key Files

### Source Code
- `src/main.go` - Application entry point
- `src/cmd/root.go` - Root command, logging setup, error handling
- `src/cmd/init.go` - All `ironbark init` subcommands
- `src/cmd/argo.go` - All `ironbark argo` subcommands (push, clone)
- `src/internal/constants/constants.go` - Environment variables and constants
- `src/internal/git/git.go` - Gitea client and Git operations via Zarf tunnel
- `src/internal/zarf/zarf.go` - Zarf cluster state and package deployment

### Build & Deployment
- `Makefile` - Build tasks (test, format, build, oci-image)
- `Dockerfile` - Multi-stage build (tooling + golang + final)
- `resources/packages/deploy/bsl-ironbark/zarf.yaml` - Core package definition

### Documentation
- `README.adoc` - User-facing documentation in AsciiDoc format
- `TODO` - Operational notes and improvement ideas

## Code Style and Conventions

### Language and Spelling
- Use **Australian English** spelling throughout (initialise, standardise, etc.)
- All documentation and code comments must use Australian spelling

### Code Formatting
- Use `gofmt` for all Go code formatting
- Run `make format` before committing
- All code comments must end with a full stop

### Documentation Style
- Use AsciiDoc format for README
- All list items must end with a full stop
- All code comments (bash, yaml, go) must end with a full stop
- Use definition lists (`term:: description`) instead of bold bullets (`* **term** - description`)
- No emoticons in documentation

### Containerisation
- Use **Podman** instead of Docker in documentation and examples
- Docker is only used in the Makefile for OCI image builds (for compatibility)

### Logging
- Use `slog` (structured logging) directly throughout the codebase
- JSON handler is configured globally via `slog.SetDefault()` in `cmd/root.go`
- Do NOT create separate logger instances in the `cmd` package
- Use `slog.Info()`, `slog.Error()`, etc. directly

### Error Handling
- Use `exitOnError(err, message)` helper in cmd package
- Panics on error (CLI tool doesn't need graceful degradation)
- Always provide context in error messages

### Comments
- All exported functions must have godoc comments
- Comments must be complete sentences ending with a full stop
- Use `// Package <name> provides...` format for package comments

### Constants and Configuration
- Environment variables are centralised in `src/internal/constants/constants.go`
- Use getter functions for environment variables with defaults
- Key environment variables:
  - `IRONBARK_DATA_DIR` - Default: `/tmp/ironbark/data` (container: `/mnt/data`)
  - `IRONBARK_INTERNAL_PACKAGES_DIR` - Default: `/tmp/ironbark/packages`

## Architecture Patterns

### CLI Structure
- Uses Cobra for command structure
- Root command in `cmd/root.go`
- Subcommands organised by feature (`init`, `argo`)
- Each command has a `newXxxCmd()` function returning `*cobra.Command`
- Execution logic in separate `xxxExec()` functions

### Git Operations
- All Git operations go through Zarf tunnel to internal Gitea
- Use `executeInGitTunnel()` helper for tunnel management
- Use Gitea SDK (`code.gitea.io/sdk/gitea`) for repository operations
- Use go-git for clone/push operations
- Extract Gitea client creation with `getGiteaClient()` helper

### Zarf Integration
- Cluster state accessed via `zarf.GetCluster()` and `zarf.GetState()`
- Package deployment via `zarf.DeployPackage()`
- Packages located via `IRONBARK_INTERNAL_PACKAGES_DIR`

### Resource Embedding
- Static resources embedded via `//go:embed` in `src/resources/resources.go`
- ArgoCD App of Apps template is embedded and deployed to cluster

## Development Workflow

### Setup
```bash
# Clone repository
git clone https://github.com/brightsparklabs/ironbark.git
cd ironbark

# Build zarf packages and extract to development location
make oci-image
id=$(podman create docker.io/brightsparklabs/ironbark:latest)
mkdir -p /tmp/ironbark/
podman cp $id:/app/resources/packages /tmp/ironbark/
podman rm ${id}
```

### Common Tasks
```bash
make              # Show all available tasks
make test         # Run unit tests
make format       # Format code with gofmt
make build        # Build binary to build/bin/ironbark
make oci-image    # Build OCI image
make clean        # Remove build artifacts
```

### Running Locally
```bash
cd src
go run . --help
go run . init all
```

### Testing
- Currently no test files exist (improvement needed)
- Run `make test` to execute tests
- Tests should be added alongside code files as `*_test.go`

## Important Patterns

### Adding New Commands
1. Create `newXxxCmd()` function returning `*cobra.Command`
2. Create `xxxExec(cmd *cobra.Command, args []string)` execution function
3. Add godoc comments to both functions
4. Add command to parent via `parentCmd.AddCommand(newXxxCmd())`
5. Use `slog.Info()` for user-facing logging
6. Use `exitOnError()` for error handling

### Working with Git
1. Use `executeInGitTunnel()` to get tunnel and credentials
2. Get Gitea client via `getGiteaClient()`
3. Use Gitea SDK for repo metadata/management
4. Use go-git for actual Git operations (clone/push)
5. Always use `repo.CloneURL` from Gitea, don't construct URLs manually

### Working with Zarf Packages
1. Define packages in `resources/packages/deploy/` or `resources/packages/mirror/`
2. Each package has a `zarf.yaml` manifest
3. Packages built during Docker image build
4. Deploy with `zarf.DeployPackage(ctx, packagePath)`

## Environment Variables

Key environment variables used:

- `IRONBARK_DATA_DIR` - Directory for application data (repos, etc.)
  - Default: `/tmp/ironbark/data`
  - Container: `/mnt/data` (mounted from host)

- `IRONBARK_INTERNAL_PACKAGES_DIR` - Directory containing Zarf packages
  - Default: `/tmp/ironbark/packages`
  - Container: `/app/resources/packages` (embedded in image)

## Common Issues and Solutions

### Missing Zarf Packages
- Ensure `IRONBARK_INTERNAL_PACKAGES_DIR` points to valid package directory
- Check packages exist with `ls ${IRONBARK_INTERNAL_PACKAGES_DIR}/deploy/`

### Git Operations Failing
- Verify Zarf Git server is running: `kubectl get pods -n zarf`
- Check tunnel connectivity
- Verify repository exists in Gitea UI

### Build Issues
- Run `go mod download` if dependencies are missing
- Ensure Go 1.25.5+ is installed
- Check Docker/Podman is available for OCI builds

## Contributing

When contributing:
1. Follow all code style conventions (Australian spelling, full stops, etc.)
2. Format code with `gofmt` before committing
3. Add godoc comments to all exported functions
4. Update README.adoc if user-facing changes
5. Ensure all tests pass (when tests exist)
6. Write clear, descriptive commit messages

## Related Documentation

- `README.adoc` - User-facing documentation
- `TODO` - Operational notes and improvement ideas
- Zarf Docs: https://docs.zarf.dev/
- ArgoCD Docs: https://argo-cd.readthedocs.io/
- Cobra Docs: https://github.com/spf13/cobra

## Notes for AI Agents

- This project has no existing test coverage - tests need to be added
- The codebase uses structured logging via `slog` - always use it directly
- Don't create `.md` files unless explicitly requested by user
- Always use Australian spelling in all output
- All code comments and documentation must end with full stops
- Use definition lists in AsciiDoc, not bold bullets
- Reference Podman in documentation, not Docker (except in Makefile)

### Testing and Debugging Guidelines

- **Never modify firewall rules without explicit user permission** - firewall changes can have security implications and should only be made when the user specifically requests it
- **Always verify test environment state before starting** - check for default routes, clean VM state, and required files before beginning installation or testing
- **When investigating issues, establish baseline first** - test with minimal configuration before adding complexity to isolate root causes
- **Don't assume configuration problems** - hardware/network/firewall issues can manifest as application configuration problems; verify the infrastructure layer first

## End to End Testing

You can test on a local VM by doing the following:

```bash
ssh bsl@localhost -p 2222 -o StrictHostKeyChecking=no
```

Password if needed is `password`.

### RKE2 Installation Testing

**Prerequisites (CRITICAL):**

1. **Set default route before installing RKE2:**
   ```bash
   sudo ip route replace default via 192.168.50.250 dev enp1s0
   ```

2. **Configure firewall for pod network (RHEL-based systems):**
   ```bash
   # Required on Rocky Linux, RHEL, CentOS for pod-to-host connectivity
   sudo firewall-cmd --zone=trusted --add-source=10.42.0.0/16 --permanent
   sudo firewall-cmd --reload
   ```

   Without this, pods cannot reach the Kubernetes API server, causing CoreDNS and other system components to fail.

**Interact with cluster:**

```bash
export KUBECONFIG=/etc/rancher/rke2/rke2.yaml
/var/lib/rancher/rke2/bin/kubectl <command>
```

### Known RKE2 Issues on Rocky Linux 9

- **Pod-to-API connectivity requires firewall configuration** - Even with Cilium kube-proxy replacement properly configured (`kubeProxyReplacement: true`, `k8sServiceHost: localhost`), pods cannot reach the Kubernetes API without adding the pod network to the firewall trusted zone.
- **This is NOT a Cilium configuration issue** - It is a firewall requirement specific to RHEL-based distributions with firewalld.
- **Symptoms:** CoreDNS 0/1 Running, pods timeout connecting to `10.43.0.1:443`, error message: `connect: no route to host`.

### RKE2 Auto-Deploy Manifests Limitations

**RKE2's Addon controller has significant limitations:**
- Strips out Deployment configurations (volume mounts, environment variables, security contexts)
- Cannot access arbitrary host filesystem paths in manifests
- Only suitable for simple resources, NOT complex operators

**For complex deployments (like Rook-Ceph), use RKE2's HelmChart CRD instead:**
- Bypasses the Addon controller
- Uses Helm's native deployment mechanism
- For air-gapped: embed chart as base64 `chartContent` (host paths don't work)
- Example: `chart: /path/to/file` fails because helm-install pod can't access host filesystem

**Image format requirements (CRITICAL):**
- **Use docker-archive format, NOT OCI layout format**
- RKE2/containerd requires explicit image name:tag in the archive for proper loading
- Correct: `skopeo copy docker://source docker-archive:/tmp/image.tar:repo/name:tag`
- Wrong: `skopeo copy docker://source oci:/tmp/image` (loses tag metadata)
- Use zstd compression for consistency: `zstd -T0 -19 /tmp/image.tar -o file.tar.zst`
- Naming: `rke2-images-<name>.linux-<arch>.tar.zst`
- **Verify format:** `zstd -d -c file.tar.zst | tar -t | head` should show `manifest.json` and `repositories`, NOT `blobs/` and `oci-layout`

**Version alignment for Helm charts:**
- **CRITICAL:** Package the exact image versions that the Helm chart expects by default
- Don't override chart image versions in Helm values unless absolutely necessary
- **To update Rook CSI versions:** Check the Rook Helm chart's `values.yaml` for the version you're using (e.g., https://github.com/rook/rook/blob/v1.15.8/deploy/charts/rook-ceph/values.yaml)
- Update the `ARG` variables at the top of the Dockerfile to match the chart's default image tags
- All image versions should be defined as `ARG` variables with comments explaining how to keep them aligned
- The VERSION.json file should document all packaged versions for traceability

### SELinux Configuration

**Do NOT set `selinux: true` in RKE2 config unless:**
- The `rke2-selinux` policy package is installed on all nodes
- Without it, RKE2 will fail to start: "process is not running in context 'container_runtime_t'"

**For air-gapped deployments:**
- Simpler to run SELinux in permissive mode (default)
- Applications like Rook-Ceph handle privileged pods via their own mechanisms (e.g., `ROOK_HOSTPATH_REQUIRES_PRIVILEGED=true`)
- This works correctly on SELinux enforcing systems without requiring RKE2 SELinux integration

### Rook-Ceph Deployment Lessons

**Image Format (CRITICAL):**
- RKE2/containerd requires **docker-archive format**, NOT OCI layout format
- Correct: `skopeo copy docker://source docker-archive:/tmp/image.tar:repo/name:tag`
- Wrong: `skopeo copy docker://source oci:/tmp/image` (loses tag metadata)
- Use `zstd -T0 -19` for compression, naming: `rke2-images-<name>.linux-<arch>.tar.zst`
- Verify format: `zstd -d -c file.tar.zst | tar -t | head` should show `manifest.json` and `repositories`, NOT `blobs/` and `oci-layout`

**Version Alignment:**
- Package the exact image versions that the Helm chart expects by default
- Check the chart's `values.yaml` for default image tags (e.g., https://github.com/rook/rook/blob/v1.15.8/deploy/charts/rook-ceph/values.yaml)
- Define all versions as `ARG` variables at the top of Dockerfile with comments
- Document all packaged versions in VERSION.json for traceability
- Do NOT override chart values unless absolutely necessary - let the chart use its defaults

**Old Ceph Data Cleanup:**
- Authentication errors (`unexpected key`) indicate old Ceph data from previous deployments
- Must clean BOTH `/dev/vdb` (or storage device) AND `/var/lib/rook/*` before redeploying
- Device wipe: `sudo dd if=/dev/zero of=/dev/vdb bs=1M count=100 && sudo wipefs -a /dev/vdb`
- Data directory: `sudo rm -rf /var/lib/rook/*`

**Resource Requirements:**
- Minimum for Rook-Ceph testing: 6 CPU cores, 12-16GB RAM
- 4 cores + 8GB RAM is insufficient for full deployment with all CSI features
- Monitor CPU/memory allocation: `kubectl describe node | grep "Allocated resources"`
- For constrained environments, disable CephFS: only RBD (block storage) needed for basic testing

**Common Deployment Issues:**
- Pause container image (`rancher/mirrored-pause:3.6`) missing after RKE2 restart/reboot
- Solution: Re-import core images: `sudo ctr -a /run/k3s/containerd/containerd.sock -n k8s.io images import /var/lib/rancher/rke2/agent/images/rke2-images-core.linux-amd64.tar.zst`
- OSD pods won't create if disk has old Ceph metadata - check OSD prepare logs
- CephCluster stuck deleting: remove finalizers from dependent resources (CephBlockPool) first
