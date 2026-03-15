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
          sh 'make deploy'
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
