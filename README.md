# Docksmith 🏗️

Docksmith is an educational, daemon-less, Docker-like container build and runtime system built from scratch in Go. 

Designed specifically for systems programming education, Docksmith strips away the massive complexity of modern container orchestrators to bare metal Linux primitives. It demonstrates exactly how images are built, how deterministic caching works, and how processes are isolated at the OS level using purely the Go standard library and Linux namespaces.

## 🌟 Core Features

- **Zero-Daemon Architecture**: There is no `dockerd` background process. The `docksmith` CLI directly manipulates state on disk and executes processes natively.
- **Deterministic Build Engine**: Implementing byte-for-byte reproducibility. Every layer generated is purely content-addressed. Timestamps are scrubbed, and ownership is normalized (`root:root`) to guarantee identical cache hashes on any machine.
- **Content-Addressable Storage (CAS)**: Layers are stored immutably in `~/.docksmith/layers/` named by their exact SHA-256 tar digest.
- **Native OS Isolation**: The runtime uses Linux Namespaces (`CLONE_NEWNS`, `CLONE_NEWPID`, `CLONE_NEWUTS`) and `chroot` to securely jail execution commands without external dependencies like `runc`.
- **Instruction Caching Pipeline**: A custom Docksmithfile parser that evaluates state, cascades cache miss signals, captures execution filesystem deltas, and hashes combinations of inputs (`ENV + WORKDIR + Previous Layer`) to prevent redundant compilation.

---

## 🛠️ Installation & Setup

Because Docksmith directly leverages Linux Namespaces, it **must** be built and run on a Linux environment (or WSL2).

```bash
# 1. Clone the repository
git clone https://github.com/ShamantN/CC_DOCKSMITH.git
cd CC_DOCKSMITH

# 2. Build the Docksmith Engine binary
go build -o docksmith ./cmd/docksmith/main.go
```

### Initializing the Base Image
Since Docksmith is built to function entirely offline, there is no `docker pull` network logic. You must ingest a root filesystem (like Alpine Linux) directly into the engine's registry:

```bash
# Navigate to the testing directory
cd testenv

# Download the Alpine minirootfs
wget -qO alpine.tar.gz https://dl-cdn.alpinelinux.org/alpine/v3.18/releases/x86_64/alpine-minirootfs-3.18.4-x86_64.tar.gz

# Import the tar directly into Docksmith (must run as sudo to establish the global ~/.docksmith registry correctly)
sudo go run import_alpine.go alpine.tar.gz
```

---

## 💻 Command Reference

All active state is managed inside the invoking user's home directory: `~/.docksmith/`.

### 1. `images`
Lists all container images currently stored in the local offline registry.
```bash
./docksmith images
```
**Output Details**: Displays the Image Name, Tag, and the computed SHA-256 Manifest Digest.

### 2. `build`
Parses a `Docksmithfile` in the target context directory, executes the instructions layer-by-layer, evaluates caching, and outputs an executable container image manifest.
```bash
# Usage: build -t <name:tag> [--no-cache] <context-directory>
sudo ./docksmith build -t myapp:v1 .
```
*(Requires `sudo` because `RUN` instructions utilize kernel namespaces to isolate the build environment).*
* **How it works**:
  * Evaluates directives like `FROM`, `WORKDIR`, `ENV`, `CMD`.
  * Computes deterministic cache keys for `COPY` and `RUN`.
  * If a `RUN` cache misses, it creates a temporary `rootfs`, intercepts the file deltas, packages them into a timestamp-zeroed tarball, and saves the new layer.
  * Optionally pass `--no-cache` to bust the cache completely.

### 3. `run`
Extracts the immutable image layers, binds them into a temporary virtual root filesystem, and launches an isolated Linux process exactly against the image configurations.
```bash
# Usage: run [-e KEY=VALUE...] <name:tag> [command]
sudo ./docksmith run myapp:v1 /bin/sh -c "echo hello from inside"
```
*(Requires `sudo` because it maps `CLONE_NEWPID` and `chroot` natively).*
* **How it works**: It pulls the image manifest, iterates over and sequentially extracts the layer hierarchy, merges any `-e` CLI flags against the internal `ENV` states, bounds the directory root, and fires the OS clone execution.

### 4. `rmi`
Removes an image manifest actively tracked from the registry.
```bash
# Usage: rmi <name:tag>
./docksmith rmi myapp:v1
```

---

## 🏗️ Architecture & Project Structure

The project strictly forces Zero-Dependencies (outside of standard Go library usages).

- `cmd/docksmith/` - Main entry-point binaries.
- `internal/cli/` - Router and parsing interface handling.
- `internal/build/` - The Docksmithfile parsing engine, execution mapping, glob resolving, and Deterministic Cache key algorithms.
- `internal/archive/` - Deterministic tarball generation scrubbing timestamps and unifying UIDs.
- `internal/image/` - JSON Manifest generation mapping content layers efficiently.
- `internal/runtime/` - OS-level runtime engine triggering direct `syscall.SysProcAttr` interactions enforcing strict Linux capability drops.
- `internal/config/` - State directory tracking (`~/.docksmith/`).

## 🎓 Educational Goals Met
- **"Docker-from-scratch"** — Unravels Container orchestration magic proving "Containers are just Linux Processes."
- **Immutable Infrastructure** — Demonstrates Content-Addressed Storage concepts.
- **Dependency Graphs** — Proves the effectiveness of DAGs in caching compilation systems.
