#!/bin/bash

set -e

echo "Starting end-to-end tests for external-dns with local provider..."

# Install kind
echo "Installing kind..."
curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.30.0/kind-linux-amd64
chmod +x ./kind
sudo mv ./kind /usr/local/bin/kind

# Create kind cluster
echo "Creating kind cluster..."
kind create cluster

# Install kubectl
echo "Installing kubectl..."
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
chmod +x kubectl
sudo mv kubectl /usr/local/bin/kubectl

# Build external-dns
echo "Building external-dns..."
make build

# Run external-dns locally in background
echo "Starting external-dns locally..."
./build/external-dns --source=service --provider=webhook --txt-owner-id=external.dns --policy=sync &
EXTERNAL_DNS_PID=$!

# Run the local provider in background
echo "Starting local provider..."
cd provider/local
go build .
./local &
LOCAL_PROVIDER_PID=$!
cd ../..

# Apply kubernetes yaml with service
echo "Applying Kubernetes service..."
kubectl apply -f e2e

# Wait for convergence
echo "Waiting for convergence (90 seconds)..."
sleep 90  # normal loop is 60 seconds, this is enough and should not cause flakes

# Check that the records are present
echo "Checking services again..."
kubectl get services

# Check that the DNS records are present
echo "Checking DNS records..."
if dig +short demo-app.external.dns | grep -q .; then
	echo "DNS record exists"
else
	echo "ERROR: DNS record not found"
	exit 1
fi

echo "End-to-end test completed!"

# Cleanup function
cleanup() {
    echo "Cleaning up..."
    if [ ! -z "$EXTERNAL_DNS_PID" ]; then
        kill $EXTERNAL_DNS_PID 2>/dev/null || true
    fi
    if [ ! -z "$LOCAL_PROVIDER_PID" ]; then
        kill $LOCAL_PROVIDER_PID 2>/dev/null || true
    fi
    kind delete cluster 2>/dev/null || true
}

# Set trap to cleanup on script exit
trap cleanup EXIT