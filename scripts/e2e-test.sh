#!/bin/bash

set -e

KO_VERSION="0.18.0"

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

# Install ko
echo "Installing ko..."
curl -sSfL "https://github.com/ko-build/ko/releases/download/v${KO_VERSION}/ko_${KO_VERSION}_linux_x86_64.tar.gz" > ko.tar.gz
tar xzf ko.tar.gz ko
chmod +x ./ko
sudo mv ko /usr/local/bin/ko

# Build external-dns
echo "Building external-dns..."
make build.image

# Run the local provider in background
# echo "Starting local provider..."
# cd provider/local
# go build .
# ./local &
# LOCAL_PROVIDER_PID=$!
# cd ../..

docker build -t webhook:v1 -f - . <<EOF
FROM golang:1.25 AS builder
WORKDIR /app
COPY . .
RUN pwd && CGO_ENABLED=0 go build -o /app/etchostprovider /app/provider/etchosts
FROM scratch
COPY --from=builder /app/etchostprovider /etchostprovider
ENTRYPOINT ["/etchostprovider"]
EOF

kind load docker-image webhook:v1
sleep 10

# # Run external-dns locally in background
# echo "Starting external-dns locally..."
# ./build/external-dns --source=service --provider=webhook --txt-owner-id=external.dns --policy=sync &
# EXTERNAL_DNS_PID=$!

# Deploy ExternalDNS to the cluster
# create kustomization on the fly to add --provider=webhook --txt-owner-id=external.dns --policy=sync to the content in the kustomize folder
echo "Deploying external-dns with custom arguments..."

# Create temporary directory for kustomization
TEMP_KUSTOMIZE_DIR=$(mktemp -d)
cp -r kustomize/* "$TEMP_KUSTOMIZE_DIR/"

# Create patch file on the fly
cat <<EOF > "$TEMP_KUSTOMIZE_DIR/deployment-args-patch.yaml"
apiVersion: apps/v1
kind: Deployment
metadata:
  name: external-dns
spec:
  template:
    spec:
      containers:
        - name: external-dns
          args:
            - --source=service
            - --provider=webhook
            - --txt-owner-id=external.dns
            - --policy=sync
        - name: webhook
          image: webhook:v1
          ports:
            - containerPort: 8888
              name: http
          volumeMounts:
            - name: hosts-file
              mountPath: /etc/hosts
          securityContext:
            privileged: true
      volumes:
      - name: hosts-file
        hostPath:
          path: /etc/hosts
          type: File
EOF

# Update kustomization.yaml to include the patch
cat <<EOF > "$TEMP_KUSTOMIZE_DIR/kustomization.yaml"
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

images:
  - name: registry.k8s.io/external-dns/external-dns
    newTag: v0.18.0

resources:
  - ./external-dns-deployment.yaml
  - ./external-dns-serviceaccount.yaml
  - ./external-dns-clusterrole.yaml
  - ./external-dns-clusterrolebinding.yaml

patchesStrategicMerge:
  - ./deployment-args-patch.yaml
EOF

# Apply the kustomization
kubectl kustomize "$TEMP_KUSTOMIZE_DIR" | kubectl apply -f -

# add a wait for the deployment to be available
kubectl wait --for=condition=available --timeout=60s deployment/external-dns || true

kubectl describe pods -l app=external-dns
kubectl describe deployment external-dns
kubectl logs -l app=external-dns

# Cleanup temporary directory
rm -rf "$TEMP_KUSTOMIZE_DIR"

# Apply kubernetes yaml with service
echo "Applying Kubernetes service..."
kubectl apply -f e2e

# Wait for convergence
echo "Waiting for convergence (90 seconds)..."
sleep 90  # normal loop is 60 seconds, this is enough and should not cause flakes

# Check that the records are present
echo "Checking services again..."
kubectl describe pods -l app=external-dns
kubectl describe deployment external-dns
kubectl logs -l app=external-dns

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
