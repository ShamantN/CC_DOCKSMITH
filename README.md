# Docksmith 🏗️

Docksmith is an educational, daemon-less, Docker-like container build and runtime system built from scratch in Go. 

Designed specifically for systems programming education, Docksmith strips away the massive complexity of modern container orchestrators to bare metal Linux primitives. It demonstrates exactly how images are built, how deterministic caching works, and how processes are isolated at the OS level using purely the Go standard library and Linux namespaces.

## 🏗️ Architectural Overview
Docksmith operates on four key principles:

1.  **Daemonless**: No background server process (`dockerd`). The CLI directly interacts with the registry and kernel.
2.  **CAS (Content-Addressable Storage)**: Every image layer is immutable and named by its SHA-256 digest. This ensures storage efficiency and data integrity.
3.  **Deterministic Caching**: Build steps are cached based on a hash of the instruction, context files, and previous layers. Normalized timestamps and ownership ensure bit-for-bit reproducibility.
4.  **OS-Level Isolation**: Containers are realized using Linux Namespaces (`PID`, `Mount`, `UTS`) and `chroot`. Hard isolation is achieved without external dependencies like `runc`.

> [!IMPORTANT]
> **Sudo Requirement**: `docksmith build` and `docksmith run` require `sudo` privileges because they utilize Linux kernel namespaces (`CLONE_NEWPID`, `CLONE_NEWNS`) and `chroot` which are restricted operations.

---

## 🛠️ Installation & Setup

```bash
# 1. Build the Docksmith Engine binary
go build -o docksmith ./cmd/docksmith/main.go
```

## 🚀 The Demo Guide (Step-by-Step)

Follow these steps to explore all core capabilities of Docksmith.

### 1. Manual Seeding (Registry Initialization)
Since Docksmith is designed to function entirely offline, you must manually seed the registry with a base image (e.g., Alpine Linux). Use the provided import tool:

```bash
cd testenv
# Assuming alpine.tar.gz exists (downloaded from https://dl-cdn.alpinelinux.org/alpine/v3.18/releases/x86_64/alpine-minirootfs-3.18.4-x86_64.tar.gz)
sudo go run import_base.go alpine.tar.gz alpine:latest
cd ..
```

### 2. Cold Build (Cache Miss)
Build the sample application for the first time. You will see cache misses for every step.

```bash
sudo ./docksmith build -t myapp:latest ./sample-app
```
*Observe that each instruction triggers an execution and layer generation.*

### 3. Warm Build (Cache Hit)
Run the exact same build command again.

```bash
sudo ./docksmith build -t myapp:latest ./sample-app
```
*Observe the `[CACHE HIT]` messages. The build should finish almost instantly.*

### 4. Cache Busting
Modify a context file to trigger a cache miss that cascades.

```bash
echo "Busting the cache!" >> ./sample-app/data.txt
sudo ./docksmith build -t myapp:latest ./sample-app
```
*Observe that the `COPY` instruction and all subsequent instructions (like `RUN` and `CMD`) result in cache misses.*

### 5. List Images
Verify that your image is registered correctly.

```bash
./docksmith images
```
*Output identifies images by Name, Tag, Image ID (digest), and Creation time.*

### 6. Run Container
Execute your isolated application.

```bash
sudo ./docksmith run myapp:latest
```
*The app should print its startup message and list the files in its internal `/app` directory.*

### 7. Override Environment Variables
Prove that runtime overrides work correctly using the `-e` flag.

```bash
sudo ./docksmith run -e KEY=NewValueOverride myapp:latest
```
*Observe in the output that `KEY` now reflects `NewValueOverride` instead of its default value.*

### 8. Process Isolation Test
Verify that the container is strictly isolated from the host filesystem.

```bash
# Create a file inside the container
sudo ./docksmith run myapp:latest /bin/sh -c 'touch /hacked.txt && ls /hacked.txt'

# Verify the file DOES NOT exist on the host
ls /hacked.txt
```
*The second command should return "No such file or directory", proving the host is untouched.*

### 9. Remove Image
Clean up the registry.

```bash
./docksmith rmi myapp:latest
./docksmith images
```
*Note: As per design, shared layers are deleted when an image is removed if reference counting is not implemented.*

---

## 🎓 Learning Objectives Met
- **Docker-from-scratch**: Unravels container magic.
- **Immutable Infrastructure**: Content-Addressed Storage concepts.
- **Dependency Graphs**: Deterministic build pipelines.
- **Linux Internals**: Direct usage of Namespaces and Chroot.
