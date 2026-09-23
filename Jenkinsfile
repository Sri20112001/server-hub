// ServerHub CI/CD — Jenkins declarative pipeline.
//
// Replaces .github/workflows/backend.yml (removed). The agent is expected to
// have Docker + Docker Compose, Go (>= 1.26) and Node 20 on PATH, and to BE
// the deploy target (same-machine `docker compose up`, like the old
// self-hosted GitHub runner).
//
// Required Jenkins "Secret text" credentials (Manage Jenkins → Credentials):
//   serverhub-postgres-password   POSTGRES_PASSWORD for compose + test DB
//   serverhub-jwt-secret          JWT_SECRET (>= 32 chars)
//   serverhub-encryption-key      SERVERHUB_ENCRYPTION_KEY (64-char hex)
//   serverhub-admin-password      ADMIN_PASSWORD (seeded on first boot)
//   serverhub-webhook-secret      GITHUB_WEBHOOK_SECRET (optional but recommended;
//                                 leave empty only if you don't use GitHub webhooks)
//
// Optional environment (configure on the job or agent if you need non-defaults):
//   FRONTEND_URL, SCAN_ROOTS, ALERT_* — fall back to docker-compose defaults.
//
// Job setup: New Item → Pipeline → "Pipeline script from SCM",
// SCM: Git, Script Path: Jenkinsfile. Trigger via webhook or polling as usual.

pipeline {
  agent any

  options {
    timestamps()
    timeout(time: 30, unit: 'MINUTES')
    disableConcurrentBuilds()
    buildDiscarder(logRotator(numToKeepStr: '20'))
  }

  environment {
    // Throwaway Postgres used ONLY by `go test` (each test creates and drops
    // its own serverhub_test_* database inside it). Mapped to host 5433 so it
    // never clashes with dev/prod Postgres on 5432.
    TEST_PG_CONTAINER = 'jenkins-serverhub-pg'
    TEST_DATABASE_URL = 'postgres://serverhub:changeme@localhost:5433/postgres?sslmode=disable'
  }

  stages {
    stage('Checkout') {
      steps {
        checkout scm
      }
    }

    stage('Start test database') {
      steps {
        sh '''
          set -e
          docker rm -f "$TEST_PG_CONTAINER" >/dev/null 2>&1 || true
          docker run -d --name "$TEST_PG_CONTAINER" \
            -e POSTGRES_DB=serverhub \
            -e POSTGRES_USER=serverhub \
            -e POSTGRES_PASSWORD=changeme \
            -p 5433:5432 \
            postgres:16-alpine
          for i in $(seq 1 30); do
            if docker exec "$TEST_PG_CONTAINER" pg_isready -U serverhub >/dev/null 2>&1; then
              echo "test postgres is ready"
              break
            fi
            sleep 2
          done
          docker exec "$TEST_PG_CONTAINER" pg_isready -U serverhub
        '''
      }
    }

    stage('Vet') {
      steps {
        dir('server') {
          sh 'go vet ./...'
        }
      }
    }

    stage('Test') {
      steps {
        dir('server') {
          sh 'go test ./... -count=1 -timeout 300s'
        }
      }
    }

    stage('Build') {
      steps {
        dir('server') {
          sh 'docker compose build'
        }
      }
    }

    stage('Deploy') {
      environment {
        POSTGRES_PASSWORD      = credentials('serverhub-postgres-password')
        JWT_SECRET             = credentials('serverhub-jwt-secret')
        SERVERHUB_ENCRYPTION_KEY = credentials('serverhub-encryption-key')
        ADMIN_PASSWORD         = credentials('serverhub-admin-password')
        GITHUB_WEBHOOK_SECRET  = credentials('serverhub-webhook-secret')
      }
      steps {
        dir('server') {
          sh '''
            set -e

            docker compose down --remove-orphans >/dev/null 2>&1 || true

            # Clean up any stale container holding host port 4000
            HOLDER="$(docker ps --format '{{.Names}} {{.Image}} {{.Ports}}' 2>/dev/null | grep '4000->4000' || true)"
            if [ -n "$HOLDER" ]; then
              echo "port holder: $HOLDER"
              IMG="$(echo "$HOLDER" | awk '{print $2}')"
              if [ "$IMG" = "serverhub" ] || [ "$IMG" = "docker.io/library/serverhub" ]; then
                NAME="$(echo "$HOLDER" | awk '{print $1}')"
                echo "stale own image container ($NAME) — stopping and removing"
                docker stop "$NAME" >/dev/null
                docker rm "$NAME" >/dev/null
              else
                echo "ERROR: host port 4000 is held by a foreign container:"
                echo "$HOLDER"
                echo "Stop it (docker stop <name>) or move our port, then re-run."
                exit 1
              fi
            fi

            if command -v ss >/dev/null 2>&1; then
              HP="$(ss -tlnp 2>/dev/null | grep ':4000 ' || true)"
              if [ -n "$HP" ]; then
                echo "ERROR: host port 4000 is held by a host process (not docker):"
                echo "$HP"
                echo "Stop it (e.g. kill <pid> / systemctl stop <service>), then re-run."
                exit 1
              fi
            fi

            docker compose up -d
            docker image prune -f
            sleep 8
            curl -s -f http://localhost:4000/health || echo "No health endpoint found or server taking longer to start"
          '''
        }
      }
    }
  }

  post {
    always {
      // Never leak the throwaway test database container between builds.
      sh 'docker rm -f "$TEST_PG_CONTAINER" >/dev/null 2>&1 || true'
    }
  }
}
