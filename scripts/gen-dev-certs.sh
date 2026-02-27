#!/usr/bin/env bash
# gen-dev-certs.sh — Generate development TLS certificates for the Unheaded service mesh.
#
# This script generates a complete PKI setup with:
#   - Root CA (10-year validity)
#   - Service certificates for all Unheaded services (1-year validity)
#   - Support for custom service lists and output directories
#
# Usage:
#   ./scripts/gen-dev-certs.sh                          # Generate all service certs
#   ./scripts/gen-dev-certs.sh [service1,service2,...]  # Generate specific services
#   CERT_DIR=/tmp/certs ./scripts/gen-dev-certs.sh      # Custom output directory
#
# Environment variables:
#   CERT_DIR      Output directory (default: configs/certs/dev)
#   CA_VALIDITY   Root CA validity (default: 87600h = 10 years)
#   CERT_VALIDITY Service cert validity (default: 8760h = 1 year)
#
# Requires: Go toolchain (builds cert-gen tool).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
CERT_DIR="${CERT_DIR:-${PROJECT_DIR}/configs/certs/dev}"
SERVICES="${1:-}"
CA_VALIDITY="${CA_VALIDITY:-87600h}"
CERT_VALIDITY="${CERT_VALIDITY:-8760h}"

echo "======================================================================="
echo "Unheaded Service Mesh — Development Certificate Generator"
echo "======================================================================="
echo "  Project:         ${PROJECT_DIR}"
echo "  Output:          ${CERT_DIR}"
echo "  CA Validity:     ${CA_VALIDITY}"
echo "  Cert Validity:   ${CERT_VALIDITY}"
echo "  Specific SVCs:   ${SERVICES:-all}"
echo ""

# Ensure output directory exists
mkdir -p "${CERT_DIR}"

# Build cert-gen tool
echo "[*] Building cert-gen tool..."
go build -o "${PROJECT_DIR}/bin/cert-gen" "${PROJECT_DIR}/cmd/cert-gen"

# Run cert-gen
# cert-gen uses a single -ttl flag for certificate validity.
ARGS="-out ${CERT_DIR} -ttl ${CERT_VALIDITY}"
if [ -n "${SERVICES}" ]; then
    ARGS="${ARGS} -services ${SERVICES}"
fi

echo "[*] Generating PKI..."
"${PROJECT_DIR}/bin/cert-gen" ${ARGS}

echo ""
echo "======================================================================="
echo "Certificate generation complete!"
echo "======================================================================="
echo ""
echo "PKI Structure:"
echo "  ${CERT_DIR}/"
echo "  ├── root-ca.crt       (Root CA certificate)"
echo "  ├── root-ca.key       (Root CA private key, 0600)"
echo "  ├── <service>.crt     (Service certificate)"
echo "  └── <service>.key     (Service private key, 0600)"
echo ""
echo "To enable mTLS in services, configure:"
echo "  PKI_DIR=\"${CERT_DIR}\""
echo ""
echo "For environment-based configuration:"
echo "  export UNHEADED_PKI_DIR=\"${CERT_DIR}\""
echo ""
echo "Services with generated certificates:"
ls -1 "${CERT_DIR}"/*.crt 2>/dev/null | sed 's/.*\//  - /' | sed 's/\.crt//' || echo "  (none yet)"
echo ""
