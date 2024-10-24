#!/bin/bash

# Set default namespace
NAMESPACE="default"

# Check if namespace is provided
if [ -n "$1" ]; then
    NAMESPACE=$1
fi

# Apply the YAML, replacing the namespace placeholder
kubectl apply -f <(curl -s https://raw.githubusercontent.com/gangyi89/linode-vlan-operator/main/deploy/vlan-operator.yaml | sed "s/\${NAMESPACE}/$NAMESPACE/") --namespace $NAMESPACE

echo ""
echo "VLAN Operator deployed to namespace: $NAMESPACE"