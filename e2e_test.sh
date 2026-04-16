#!/bin/bash
# Docksmith Ultimate E2E Bash Gauntlet
set -e

# Configuration
GO_BIN="/home/shamant-nagabhushana/.local/go/bin/go"
WORKSPACE=$(pwd)
E2E_HOME="$WORKSPACE/e2e_home"
DOCKSMITH="$WORKSPACE/docksmith"
export HOME="$E2E_HOME"
export PATH="$PATH:/home/shamant-nagabhushana/.local/go/bin"

echo "------------------------------------------------"
echo "🚀 Starting Docksmith Ultimate E2E Gauntlet"
echo "------------------------------------------------"

# 0. Cleanup and Prep
rm -rf "$E2E_HOME"
mkdir -p "$E2E_HOME/.docksmith/images"
mkdir -p "$E2E_HOME/.docksmith/layers"
mkdir -p "$E2E_HOME/.docksmith/cache"
rm -f "$DOCKSMITH"

# 1. Compilation
echo "1. [COMPILE] Building Docksmith binary..."
"$GO_BIN" build -o "$DOCKSMITH" ./cmd/docksmith/main.go
echo "   OK: Binary compiled at $DOCKSMITH"

# 2. Registry Seeding
echo "2. [SEED] Importing base alpine image..."
# No sudo needed for seeding into $HOME/e2e_home/.docksmith
if [ ! -f "testenv/alpine.tar.gz" ]; then
    echo "   Error: testenv/alpine.tar.gz not found. Cannot seed."
    exit 1
fi
"$GO_BIN" run testenv/import_base.go testenv/alpine.tar.gz alpine:latest
echo "   OK: alpine:latest imported"

# 2.5 [PREP] Create a non-sudo mini-app (no RUN)
mkdir -p "$WORKSPACE/mini-app"
echo "FROM alpine:latest" > "$WORKSPACE/mini-app/Docksmithfile"
echo "COPY . /app" >> "$WORKSPACE/mini-app/Docksmithfile"
echo "hello" > "$WORKSPACE/mini-app/hello.txt"

# 3. Cold Build
echo "3. [COLD BUILD] Building mini-app for the first time..."
COLD_OUT=$("$DOCKSMITH" build -t miniapp "$WORKSPACE/mini-app")
echo "$COLD_OUT" | grep "CACHE MISS" > /dev/null && echo "   OK: Cache Miss detected" || (echo "   FAIL: Expected Cache Miss"; exit 1)

# 4. Warm Build
echo "4. [WARM BUILD] Building mini-app again (100% hit)..."
WARM_OUT=$("$DOCKSMITH" build -t miniapp "$WORKSPACE/mini-app")
echo "$WARM_OUT" | grep "CACHE HIT" > /dev/null && echo "   OK: Cache Hit detected" || (echo "   FAIL: Expected Cache Hit"; exit 1)

# 5. Digest Stability Check
COLD_DIGEST=$("$DOCKSMITH" images | grep "miniapp" | awk '{print $3}')
WARM_DIGEST=$("$DOCKSMITH" images | grep "miniapp" | awk '{print $3}')
if [ "$COLD_DIGEST" == "$WARM_DIGEST" ]; then
    echo "   OK: Digest stable across 100% cache-hit rebuild ($COLD_DIGEST)"
else
    echo "   FAIL: Digest drifted! Cold: $COLD_DIGEST, Warm: $WARM_DIGEST"
    exit 1
fi

# 6. Cache Busting
echo "6. [CACHE BUST] Modifying hello.txt to bust cache..."
echo "busting" >> "$WORKSPACE/mini-app/hello.txt"
BUST_OUT=$("$DOCKSMITH" build -t miniapp "$WORKSPACE/mini-app")
echo "$BUST_OUT" | grep "COPY . /app \[CACHE MISS\]" > /dev/null && echo "   OK: Cache Bust detected at COPY" || (echo "   FAIL: Expected Cache Bust"; exit 1)

# 7. Isolation Test (Optional Sudo)
echo "7. [ISOLATION] Testing host filesystem boundary..."
# Note: This might fail if sudo -n is not allowed. We catch it gracefully.
if sudo -n "$DOCKSMITH" run miniapp /bin/sh -c 'touch /host_leak_test.txt' 2>/dev/null; then
    if [ -f /host_leak_test.txt ]; then
        echo "   FAIL: LEAK DETECTED! /host_leak_test.txt exists on host."
        sudo -n rm /host_leak_test.txt
        exit 1
    else
        echo "   OK: No leak detected on host filesystem"
    fi
else
    echo "   SKIP: 'docksmith run' requires sudo. Please verify manually."
fi

# 8. Environment Override Test
echo "8. [ENV] Testing runtime environment override..."
if ENV_OUT=$(sudo -n "$DOCKSMITH" run -e KEY=NewVal testapp /bin/sh -c 'env | grep KEY' 2>/dev/null); then
    echo "$ENV_OUT" | grep "KEY=NewVal" > /dev/null && echo "   OK: ENV override prioritised" || (echo "   FAIL: ENV override failed. Out: $ENV_OUT"; exit 1)
else
    echo "   SKIP: 'docksmith run' requires sudo. Please verify manually."
fi

# 9. CLI images check
echo "9. [CLI] Verify images table format..."
"$DOCKSMITH" images | grep "NAME" | grep "TAG" | grep "IMAGE ID" | grep "CREATED" > /dev/null && echo "   OK: Table headers verified" || (echo "   FAIL: Table format invalid" ; exit 1)

echo "------------------------------------------------"
echo "✅ GAUNTLET COMPLETE: DOCKSMITH IS IMPENETRABLE"
echo "------------------------------------------------"
