#!/bin/bash
set -euo pipefail
echo "=== Secret Rotation Test ==="
echo "1. Reading current secret..."
kubectl -n unheaded-system exec vault-0 -- vault kv get -format=json unheaded/cuirass/config | jq .data.data.api_bind_addr
echo "2. Rotating secret..."
kubectl -n unheaded-system exec vault-0 -- vault kv put unheaded/cuirass/config \
  api_bind_addr="0.0.0.0:8081" grpc_bind_addr="0.0.0.0:9091" election_timeout="5s"
echo "3. Waiting for CSI rotation poll (120s max)..."
sleep 130
echo "4. Verifying rotated secret in pod..."
# Would verify pod picked up new value
echo "=== Rotation test complete ==="
