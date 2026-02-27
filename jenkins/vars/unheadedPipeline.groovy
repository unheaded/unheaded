// Unheaded Shared Pipeline Library
// Groovy DSL functions for common pipeline operations
// Usage: @Library('unheadedPipeline') _

// Go setup and caching
def goSetup(String version = '1.24') {
  node {
    sh '''
      echo "=== Setting up Go ${version} ==="
      go version
      go mod download
      go mod verify
      echo "✓ Go setup complete"
    '''
  }
}

// Rust setup (stable + nightly)
def rustSetup(String stable = 'stable', String nightly = 'nightly-2025-06-01') {
  node {
    sh '''
      echo "=== Setting up Rust toolchains ==="
      echo "Stable: ${stable}"
      echo "Nightly: ${nightly}"
      echo "Note: Requires rustup in environment"
      echo "TODO: Install via rustup or docker image"
    '''
  }
}

// Security scanning (govulncheck + gosec)
def securityScan() {
  node {
    sh '''
      echo "=== Security Scanning ==="

      echo "Running govulncheck..."
      go install golang.org/x/vuln/cmd/govulncheck@latest
      govulncheck ./... || EXIT_CODE=$?

      echo "Running gosec..."
      go install github.com/securego/gosec/v2/cmd/gosec@latest
      gosec -fmt json -out gosec-report.json ./... || true
      gosec -fmt text ./...

      echo "✓ Security scan complete"
    '''
  }
}

// SBOM generation
def sbomGenerate(String version = 'dev') {
  node {
    sh '''
      echo "=== SBOM Generation ==="
      ./scripts/generate-sbom.sh ${version}
      ls -lh sbom-results/
      echo "✓ SBOM generated"
    '''
  }
}

// Coverage gate (fail if below threshold)
def coverageGate(double threshold = 60.0) {
  node {
    sh '''
      echo "=== Coverage Gate ==="
      COVERAGE=$(go tool cover -func=coverage.out | tail -1 | awk '{print $NF}' | tr -d '%')
      echo "Coverage: ${COVERAGE}%"

      if (( $(echo "${COVERAGE} < ${threshold}" | bc -l) )); then
        echo "ERROR: Coverage ${COVERAGE}% below threshold ${threshold}%"
        exit 1
      fi
      echo "✓ Coverage gate passed"
    '''
  }
}

// Run tests with coverage
def testGo(String args = '-race -cover -timeout 300s') {
  node {
    sh '''
      echo "=== Running Go tests ==="
      go test -v ${args} -coverprofile=coverage.out ./...
      go tool cover -func=coverage.out | tail -1
      echo "✓ Tests complete"
    '''
  }
}

// Lint Go code
def lintGo() {
  node {
    sh '''
      echo "=== Linting Go code ==="
      go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
      golangci-lint run ./... --timeout=5m
      echo "✓ Linting complete"
    '''
  }
}

// Build Go binaries
def buildGo(String version = 'dev', String output = 'bin') {
  node {
    sh '''
      echo "=== Building Go binaries ==="
      mkdir -p ${output}

      go build \
        -ldflags="-s -w -X main.Version=${version}" \
        -o ${output}/unheaded-daemon ./cmd/unheaded-daemon

      go build \
        -ldflags="-s -w -X main.Version=${version}" \
        -o ${output}/dashboard-backend ./cmd/dashboard-backend

      go build \
        -ldflags="-s -w -X main.Version=${version}" \
        -o ${output}/kanban-app ./cmd/kanban-app

      ls -lh ${output}/
      echo "✓ Build complete"
    '''
  }
}

// Archive artifacts
def archiveResults(String artifacts = 'bin/**/*,coverage.out,*-report.json') {
  node {
    archiveArtifacts artifacts: artifacts, allowEmptyArchive: true
  }
}

// Notify status
def notifyStatus(String status = 'success', String channel = '#unheaded-ci') {
  node {
    if (status == 'success') {
      echo "✅ Pipeline succeeded"
    } else {
      echo "❌ Pipeline failed"
    }
    // TODO: Integrate Slack/Teams notification
    // slackSend(channel: channel, message: "Build ${status}", color: (status == 'success' ? 'good' : 'danger'))
  }
}

// Validate release version
def validateVersion(String version) {
  node {
    sh '''
      echo "=== Validating version ==="
      if ! echo "${version}" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+'; then
        echo "ERROR: Version must be in format vX.Y.Z"
        exit 1
      fi
      echo "✓ Version valid: ${version}"
    '''
  }
}

// Generate release artifacts
def releaseArtifacts(String version, String dryRun = 'true') {
  node {
    sh '''
      echo "=== Generating Release Artifacts ==="
      mkdir -p dist

      # Build for multiple platforms
      for GOOS in linux darwin; do
        for GOARCH in amd64 arm64; do
          echo "Building ${GOOS}/${GOARCH}..."
          GOOS=${GOOS} GOARCH=${GOARCH} go build \
            -ldflags="-s -w -X main.Version=${version}" \
            -o dist/unheaded-${GOOS}-${GOARCH} ./cmd/unheaded-daemon
        done
      done

      # Generate checksums
      cd dist
      sha256sum * > checksums-${version}.txt
      cd ..

      echo "✓ Release artifacts ready"
      ls -lh dist/
    '''
  }
}

// Return function definitions for pipeline use
return this
