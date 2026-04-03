// Unheaded Kingdom - Main Jenkinsfile (Declarative Pipeline)
// CI/CD Hardening: Build, Test, Security, SPDX, SBOM, Deploy
// Purpose: Core build, test, security, and SBOM generation pipeline

pipeline {
  agent {
    docker {
      image 'golang:1.24-alpine'
      args '-v /var/run/docker.sock:/var/run/docker.sock'
    }
  }

  options {
    timestamps()
    timeout(time: 30, unit: 'MINUTES')
    disableConcurrentBuilds()
    buildDiscarder(logRotator(numToKeepStr: '20', artifactNumToKeepStr: '10'))
  }

  parameters {
    string(name: 'VERSION', defaultValue: 'dev', description: 'Version tag for artifacts')
    booleanParam(name: 'RUN_SECURITY', defaultValue: true, description: 'Run security scans')
    booleanParam(name: 'RUN_SBOM', defaultValue: true, description: 'Generate SBOM')
    booleanParam(name: 'RUN_DEPLOY', defaultValue: false, description: 'Deploy after build (main branch only)')
  }

  environment {
    GO_VERSION = '1.24'
    GOFLAGS = '-v -mod=readonly'
    CGO_ENABLED = '0'
    REGISTRY = 'ghcr.io'
    IMAGE_NAME = "${REGISTRY}/${GIT_REPOSITORY_OWNER}/${GIT_REPOSITORY}"
  }

  stages {
    stage('Dependencies') {
      steps {
        script {
          echo "=== Setting up Go dependencies ==="
          sh '''
            go version
            go mod download
            go mod verify
          '''
        }
      }
    }

    stage('Build') {
      steps {
        script {
          echo "=== Building binaries ==="
          sh '''
            mkdir -p bin
            go build -ldflags="-s -w -X main.Version=${VERSION}" -o bin/unheaded-daemon ./cmd/unheaded-daemon
            go build -ldflags="-s -w -X main.Version=${VERSION}" -o bin/dashboard-backend ./cmd/dashboard-backend
            go build -ldflags="-s -w -X main.Version=${VERSION}" -o bin/kanban-app ./cmd/kanban-app
            echo "Binaries built:"
            ls -lh bin/
          '''
        }
      }
    }

    stage('Static Analysis') {
      parallel {
        stage('Vet') {
          steps {
            script {
              echo "=== Running go vet ==="
              sh '''
                go vet ./...
                echo "Vet passed"
              '''
            }
          }
        }
        stage('Lint') {
          steps {
            script {
              echo "=== Running golangci-lint ==="
              sh '''
                go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
                golangci-lint run ./... --timeout=5m
                echo "Lint passed"
              '''
            }
          }
        }
        stage('SPDX Headers') {
          steps {
            script {
              echo "=== Checking SPDX license headers ==="
              sh '''
                missing=$(find . -name "*.go" -not -path "./.git/*" -not -path "./vendor/*" \
                  -exec grep -L "SPDX-License-Identifier" {} + 2>/dev/null | head -20)
                if [ -n "$missing" ]; then
                  echo "ERROR: Files missing SPDX-License-Identifier header:"
                  echo "$missing"
                  exit 1
                fi
                echo "SPDX header check passed"
              '''
            }
          }
        }
      }
    }

    stage('Test') {
      steps {
        script {
          echo "=== Running unit tests with race detection and coverage ==="
          sh '''
            go test -v -race -count=1 -timeout 300s -coverprofile=coverage.out ./...
            go tool cover -func=coverage.out | tail -1

            COVERAGE=$(go tool cover -func=coverage.out | tail -1 | awk '{print $NF}' | tr -d '%')
            echo "Total coverage: ${COVERAGE}%"
            if [ "$(echo "$COVERAGE < 60" | bc -l)" = "1" ]; then
              echo "ERROR: Coverage ${COVERAGE}% is below 60% threshold"
              exit 1
            fi
          '''
        }
      }
    }

    stage('Security') {
      when {
        expression { return params.RUN_SECURITY }
      }
      parallel {
        stage('govulncheck') {
          steps {
            script {
              echo "=== Running govulncheck ==="
              sh '''
                go install golang.org/x/vuln/cmd/govulncheck@latest
                govulncheck ./...
              '''
            }
          }
        }
        stage('gosec') {
          steps {
            script {
              echo "=== Running gosec ==="
              sh '''
                go install github.com/securego/gosec/v2/cmd/gosec@latest
                gosec -fmt json -out gosec-report.json ./... || true
                gosec -fmt text ./...
              '''
            }
          }
        }
      }
    }

    stage('SBOM') {
      when {
        expression { return params.RUN_SBOM }
      }
      steps {
        script {
          echo "=== Generating SBOM ==="
          sh '''
            if [ -x scripts/generate-sbom.sh ]; then
              ./scripts/generate-sbom.sh ${VERSION}
              ls -lh sbom-results/ 2>/dev/null || true
            else
              echo "SBOM script not found, skipping"
            fi
          '''
        }
      }
    }

    stage('License Check') {
      steps {
        script {
          echo "=== Running license compliance checks ==="
          sh '''
            go install github.com/google/go-licenses@latest
            go-licenses check ./... --allowed_licenses=MIT,Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC
            echo "Go license check passed"
          '''
          echo "=== Running GPL boundary verification ==="
          sh '''
            if [ -x scripts/verify-gpl-boundary.sh ]; then
              scripts/verify-gpl-boundary.sh
            else
              chmod +x scripts/verify-gpl-boundary.sh 2>/dev/null && scripts/verify-gpl-boundary.sh || echo "GPL boundary script not available"
            fi
          '''
        }
      }
      post {
        always {
          archiveArtifacts artifacts: 'sbom-results/gpl-boundary-report.txt', allowEmptyArchive: true
        }
      }
    }

    stage('Package (.deb)') {
      steps {
        script {
          echo "=== Creating .deb packages ==="
          sh '''
            mkdir -p build/deb
            SERVICES="unheaded-daemon dashboard-backend kanban-app"

            for svc_bin in $SERVICES; do
              PKG_NAME="unheaded-${svc_bin}"
              PKG_DIR="build/deb/${PKG_NAME}_${VERSION}"

              mkdir -p ${PKG_DIR}/DEBIAN
              mkdir -p ${PKG_DIR}/opt/unheaded/bin
              mkdir -p ${PKG_DIR}/etc/systemd/system

              # Copy binary
              if [ -f "bin/${svc_bin}" ]; then
                cp "bin/${svc_bin}" ${PKG_DIR}/opt/unheaded/bin/
                chmod 755 ${PKG_DIR}/opt/unheaded/bin/${svc_bin}
              fi

              # Control file
              cat > ${PKG_DIR}/DEBIAN/control <<EOF
Package: ${PKG_NAME}
Version: ${VERSION}
Architecture: amd64
Maintainer: Stevie Bellis <stevie@bellis.tech>
Description: Unheaded ${svc_bin} service
 Part of the Unheaded Kingdom infrastructure platform.
 https://github.com/unheaded/unheaded
Depends: libc6
Section: admin
Priority: optional
EOF

              # Systemd unit
              cat > ${PKG_DIR}/etc/systemd/system/${PKG_NAME}.service <<EOF
[Unit]
Description=Unheaded ${svc_bin}
After=network.target

[Service]
Type=simple
User=unheaded
Group=unheaded
ExecStart=/opt/unheaded/bin/${svc_bin}
Restart=always
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/unheaded /var/log/unheaded

[Install]
WantedBy=multi-user.target
EOF

              # Post-install script
              cat > ${PKG_DIR}/DEBIAN/postinst <<'POSTINST'
#!/bin/sh
set -e
# Create unheaded user if it doesn't exist
if ! id -u unheaded > /dev/null 2>&1; then
  useradd --system --home-dir /opt/unheaded --shell /usr/sbin/nologin unheaded
fi
mkdir -p /var/lib/unheaded /var/log/unheaded /etc/unheaded
chown unheaded:unheaded /var/lib/unheaded /var/log/unheaded
systemctl daemon-reload
POSTINST
              chmod 755 ${PKG_DIR}/DEBIAN/postinst

              # Pre-remove script
              cat > ${PKG_DIR}/DEBIAN/prerm <<PRERM
#!/bin/sh
set -e
systemctl stop ${PKG_NAME} 2>/dev/null || true
systemctl disable ${PKG_NAME} 2>/dev/null || true
PRERM
              chmod 755 ${PKG_DIR}/DEBIAN/prerm

              dpkg-deb --build ${PKG_DIR}
              echo "  Built: ${PKG_NAME}_${VERSION}.deb"
            done

            echo "=== All packages ==="
            ls -lh build/deb/*.deb 2>/dev/null
          '''
        }
      }
      post {
        success {
          archiveArtifacts artifacts: 'build/deb/*.deb', allowEmptyArchive: true
        }
      }
    }

    stage('Publish to APT Repo') {
      when {
        branch 'main'
      }
      steps {
        script {
          echo "=== Publishing to local APT repository ==="
          sh '''
            APT_REPO="/var/lib/apt-repo"
            CODENAME="noble"
            if [ -d "$APT_REPO" ] && which reprepro > /dev/null 2>&1; then
              for deb in build/deb/*.deb; do
                reprepro -b ${APT_REPO} includedeb ${CODENAME} ${deb} || echo "Publish failed: ${deb}"
              done
              echo "=== Published packages ==="
              reprepro -b ${APT_REPO} list ${CODENAME}
            else
              echo "APT repo not configured — skipping publish"
              echo "Run runbooks/infra/apt-repo-server.yaml to set up"
            fi
          '''
        }
      }
    }

    stage('Deploy') {
      when {
        allOf {
          branch 'main'
          expression { return params.RUN_DEPLOY }
        }
      }
      steps {
        script {
          echo "=== Deploying Unheaded Kingdom ==="
          sh '''
            # Deploy via apt on target hosts
            # ssh govan@east "sudo apt update && sudo apt install -y unheaded-wotan unheaded-daemon"
            make deploy
          '''
        }
      }
    }
  }

  post {
    always {
      script {
        echo "=== Archiving artifacts ==="
        archiveArtifacts artifacts: 'bin/**/*,coverage.out,sbom-results/**/*,gosec-report.json', allowEmptyArchive: true
      }
    }
    success {
      script {
        echo "Pipeline succeeded"
      }
    }
    failure {
      script {
        echo "Pipeline failed"
      }
    }
    cleanup {
      script {
        cleanWs()
      }
    }
  }
}
