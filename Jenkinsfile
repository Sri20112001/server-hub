// ServerHub CI/CD — Jenkins declarative pipeline.
//
// Replaces .github/workflows/backend.yml (removed). The agent is expected to
// have Docker, Go (>= 1.26) and Node 22 LTS (min 20.19 for Expo SDK 57) on PATH, and to BE the deploy target
// (same-machine `docker compose up`, like the old self-hosted GitHub runner).
// Works both on a bare-metal agent and from a containerized Jenkins with the
// Docker socket mounted: the pipeline self-provisions a Compose binary if the
// `docker compose` plugin is missing, and reaches the throwaway test Postgres
// via its bridge IP (localhost inside a Jenkins container is the wrong host).
//
// Required Jenkins "Secret text" credentials (Manage Jenkins → Credentials):
//   serverhub-postgres-password   POSTGRES_PASSWORD for compose + test DB
//   serverhub-jwt-secret          JWT_SECRET (>= 32 chars)
//   serverhub-encryption-key      SERVERHUB_ENCRYPTION_KEY (64-char hex)
//   serverhub-admin-password      ADMIN_PASSWORD (seeded on first boot)
//   serverhub-webhook-secret      GITHUB_WEBHOOK_SECRET (optional but recommended;
//                                 leave empty only if you don't use GitHub webhooks)
//   expo-token                    EXPO_TOKEN for EAS Android builds (only needed
//                                 when BUILD_MOBILE=true; see below)
//
// Optional environment (configure on the job or agent if you need non-defaults):
//   FRONTEND_URL, SCAN_ROOTS, ALERT_* — fall back to docker-compose defaults.
//   EXPO_PUBLIC_API_URL — backend URL baked into the mobile EAS build
//                         (default: http://localhost:4000).
//   BUILD_MOBILE — set to 'true' to run the Mobile EAS Build stage
//                  (default 'false': Mobile CI still runs type-check + lint,
//                  only the cloud EAS build is skipped).
//
// Mobile: Expo SDK 57 on Node 22+ (min 20.19). Mobile CI runs
// `npm ci` (lockfile-pinned) + `tsc --noEmit` + flat-config `eslint`.
// If you add a dependency locally with --legacy-peer-deps, commit the
// resulting package-lock.json so `npm ci` stays reproducible.
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
    // its own serverhub_test_* database inside it). Published on host 5433 so
    // it never clashes with dev/prod Postgres on 5432. The Test stage
    // overrides TEST_DATABASE_URL with the container bridge IP written by the
    // Start test database stage; this default is only a fallback.
    TEST_PG_CONTAINER = 'jenkins-serverhub-pg'
    TEST_DATABASE_URL = 'postgres://serverhub:changeme@localhost:5433/postgres?sslmode=disable'
    // Base compose file plus the Jenkins override (named volume instead of
    // the ./data bind mount, which the host daemon cannot resolve from
    // inside a Jenkins container). Used by Build and Deploy stages.
    COMPOSE_FILE = 'docker-compose.yml:docker-compose.jenkins.yml'
    // Set to 'true' on the job to also run a cloud EAS Android build.
    BUILD_MOBILE = 'true'
    // Baked into the JS bundle at build time (EXPO_PUBLIC_* are static).
    // For EAS cloud builds this MUST be a URL the phone can reach directly
    // (public IP/domain or VPN) — localhost/LAN IPs only work for local
    // `npx expo start` sessions on the same network.
    EXPO_PUBLIC_API_URL = 'http://localhost:4000'
  }

  stages {
    stage('Checkout') {
      steps {
        checkout scm
      }
    }

    stage('Prepare') {
      steps {
        sh '''
          set -e
          # Ensure a `docker compose` implementation exists. Prefer the plugin;
          # otherwise fetch the standalone binary once into the workspace.
          if docker compose version >/dev/null 2>&1; then
            echo "docker compose" > .jenkins-compose
          else
            mkdir -p .jenkins-bin
            if [ ! -x .jenkins-bin/docker-compose ]; then
              curl -SL "https://github.com/docker/compose/releases/download/v2.29.7/docker-compose-linux-$(uname -m)" \
                -o .jenkins-bin/docker-compose
              chmod +x .jenkins-bin/docker-compose
            fi
            echo "$WORKSPACE/.jenkins-bin/docker-compose" > .jenkins-compose
          fi
          echo "compose: $(cat .jenkins-compose)"
          $(cat .jenkins-compose) version
        '''
      }
    }

    stage('Client CI') {
      steps {
        dir('client') {
          sh '''
            set -e
            npm ci --no-audit --no-fund
            npm run lint
            npm run build
          '''
        }
      }
    }

    stage('Mobile CI') {
      steps {
        dir('mobile') {
          sh '''
            set -e
            npm ci --no-audit --no-fund
            npm run type-check
            npm run lint
          '''
        }
      }
    }

    stage('Mobile EAS Build') {
      when {
        environment name: 'BUILD_MOBILE', value: 'true'
      }
      steps {
        dir('mobile') {
          withCredentials([string(credentialsId: 'expo-token', variable: 'EXPO_TOKEN')]) {
            sh '''
              set -e
              export EXPO_TOKEN="$EXPO_TOKEN"
              export EXPO_PUBLIC_API_URL="$EXPO_PUBLIC_API_URL"
              npx -y eas-cli@latest build --platform android --profile preview --non-interactive
            '''
          }
        }
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
          # Reachable address for the tests: inside a Jenkins container,
          # localhost is the Jenkins container itself, so use the bridge IP
          # (also reachable from a bare-metal agent on Linux).
          PG_IP="$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$TEST_PG_CONTAINER")"
          echo "postgres://serverhub:changeme@${PG_IP}:5432/postgres?sslmode=disable" > .jenkins-test-db-url
          echo "testdb: $(cat .jenkins-test-db-url)"
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
          sh '''
            set -e
            if [ -f "$WORKSPACE/.jenkins-test-db-url" ]; then
              export TEST_DATABASE_URL="$(cat "$WORKSPACE/.jenkins-test-db-url")"
            fi
            echo "TEST_DATABASE_URL=$TEST_DATABASE_URL"
            go test ./... -count=1 -timeout 300s
          '''
        }
      }
    }

    stage('Build') {
      steps {
        dir('server') {
          sh '''
            set -e
            COMPOSE="$(cat "$WORKSPACE/.jenkins-compose")"
            $COMPOSE build
          '''
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
        // Host port for the compose Postgres. 5432 is taken on this host by a
        // non-Docker listener, so we bind 5434 instead. The app itself talks
        // to Postgres over the compose network (postgres:5432), unaffected.
        POSTGRES_PORT          = '5434'
      }
      steps {
        dir('server') {
          sh '''
            set -e
            COMPOSE="$(cat "$WORKSPACE/.jenkins-compose")"

            $COMPOSE down --remove-orphans >/dev/null 2>&1 || true

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

            $COMPOSE up -d
            docker image prune -f
            sleep 8
            # From a containerized Jenkins, localhost is the wrong host, so
            # fall back to probing from inside the app container (busybox wget).
            ( curl -s -f http://localhost:4000/health 2>/dev/null \
              || docker exec serverhub wget -q -O- http://localhost:4000/health ) \
              || echo "No health endpoint found or server taking longer to start"
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
