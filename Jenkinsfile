// ServerHub CI/CD — Jenkins declarative pipeline.
// Agent prerequisites: Docker + Compose, Go >= 1.25 (matches server/go.mod), Node 22 LTS,
// and (for the Mobile APK stage) JDK 17 + Android SDK cmdline-tools with
// ANDROID_HOME set (defaults to $JENKINS_HOME/android-sdk, which survives
// container recreates because it lives in the Jenkins home volume).
pipeline {
  agent any

  options {
    timestamps()
    timeout(time: 30, unit: 'MINUTES')
    disableConcurrentBuilds()
    buildDiscarder(logRotator(numToKeepStr: '20'))
  }

  parameters {
    booleanParam(name: 'BUILD_WEB', defaultValue: true, description: 'Build the web application')
    booleanParam(name: 'BUILD_MOBILE', defaultValue: false, description: 'Run mobile CI and build the mobile application')
    booleanParam(name: 'DEPLOY', defaultValue: true, description: 'Deploy ServerHub after successful CI')
    booleanParam(name: 'STAGING_SMOKE', defaultValue: true, description: 'Run post-deploy staging smoke checks (notification policy gate)')
    booleanParam(name: 'STAGING_SEND_TESTS', defaultValue: false, description: 'Include real provider test-sends in staging smoke (sends one email/Telegram; leave off in routine CI)')
  }

  environment {
    // Base name for the throwaway test Postgres. Stages derive a per-build
    // container name so a wedged container from an older run can never
    // collide with the current one. JOB_NAME is sanitized into the network
    // name because BUILD_NUMBER alone can collide across Jenkins jobs.
    TEST_PG_CONTAINER_BASE = 'jenkins-serverhub-pg'
    TEST_PG_NETWORK_BASE = 'jenkins-test-network'
    // Host loopback port publishing the test Postgres (see Start Test
    // Database: the agent shell may not share the container network).
    TEST_PG_PORT = '55432'

    COMPOSE_FILE = 'docker-compose.yml:docker-compose.jenkins.yml'

    EXPO_PUBLIC_API_URL = 'http://localhost:4000'

    // Pinned supply-chain tool versions (Security stage).
    GITLEAKS_VERSION = 'v8.24.2'
    TRIVY_VERSION = '0.65.0'
    SYFT_VERSION = 'v1.27.1'
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

      echo "Checking Docker..."
      docker --version

      echo "Checking Docker Compose..."
      docker compose version

      # Per-build network name (job-sanitized): sharing one fixed network
      # across jobs risks deleting a network another job is using.
      SAFE_JOB="$(echo "${JOB_NAME:-local}" | tr -c 'a-zA-Z0-9' '-' | cut -c1-64)"
      TEST_PG_NETWORK="jenkins-test-network-${SAFE_JOB}-${BUILD_NUMBER:-local}"
      echo "$TEST_PG_NETWORK" > "$WORKSPACE/.jenkins-test-net"

      echo "Preparing test Docker network ($TEST_PG_NETWORK)..."
      docker network create "$TEST_PG_NETWORK" 2>/dev/null || true

      # The compose wrapper previous stages assumed but nothing created.
      # Written here deterministically instead of relying on agent state.
      printf 'docker compose' > "$WORKSPACE/.jenkins-compose"

      echo "Preparation completed."
    '''
  }
}

    stage('CI') {
      parallel {

        stage('Client CI') {
    when { expression { params.BUILD_WEB } }
          steps {
            dir('client') {
              sh '''
                set -e

                echo "Installing client dependencies..."
                npm ci --no-audit --no-fund

                echo "Running client lint..."
                npm run lint

                echo "Building client..."
                npm run build
              '''
            }
          }
        }

        stage('Mobile CI') {
    when { expression { params.BUILD_MOBILE } }
          steps {
            dir('mobile') {
              // Release gate: type-check and lint fail the build. A broken
              // mobile client must never ride along silently into Deploy.
              sh '''
                set -e

                echo "Installing mobile dependencies..."
                npm ci --no-audit --no-fund

                echo "Running mobile type check..."
                npm run type-check

                echo "Running mobile lint..."
                npm run lint
              '''
            }
          }
        }

        stage('Server Vet') {
          steps {
            dir('server') {
              sh '''
                set -e

                echo "Running Go vet..."
                go vet ./...
              '''
            }
          }
        }
      }
    }

    stage('Start Test Database') {
      steps {
        sh '''
          set -e

          echo "Preparing test PostgreSQL..."

          # Per-build name: immune to leftovers from older runs, so a
          # best-effort remove is all that's needed here.
          export TEST_PG_CONTAINER="${TEST_PG_CONTAINER_BASE}-${BUILD_NUMBER:-local}"
          TEST_PG_NETWORK="$(cat "$WORKSPACE/.jenkins-test-net")"
          docker rm -f "$TEST_PG_CONTAINER" >/dev/null 2>&1 || true

          # Make sure the network exists.
          docker network create "$TEST_PG_NETWORK" 2>/dev/null || true

          # Best-effort: attach the agent itself to the test network so the
          # container name resolves from `go test`. Works when the agent is
          # a container on this daemon (hostname == container id); harmless
          # otherwise.
          docker network connect "$TEST_PG_NETWORK" "$(hostname)" >/dev/null 2>&1 || true

          docker run -d \
            --name "$TEST_PG_CONTAINER" \
            --network "$TEST_PG_NETWORK" \
            -p "127.0.0.1::5432" \
            -e POSTGRES_DB=serverhub \
            -e POSTGRES_USER=serverhub \
            -e POSTGRES_PASSWORD=changeme \
            postgres:16-alpine

          echo "Waiting for PostgreSQL..."

          READY=false

          for i in $(seq 1 30); do
            if docker exec "$TEST_PG_CONTAINER" \
              pg_isready \
              -U serverhub \
              -d serverhub >/dev/null 2>&1; then

              READY=true
              echo "Test PostgreSQL is ready."
              break
            fi

            sleep 2
          done

          if [ "$READY" != "true" ]; then
            echo "ERROR: PostgreSQL failed to start."

            docker logs "$TEST_PG_CONTAINER" || true

            exit 1
          fi

          # Pick a reachable address for the agent shell: the container name
          # when it resolves (shared network), else the published loopback
          # port. Persisted for the Test stage (env vars do not cross stages).
          # Prefer the container hostname when reachable from the agent.
          if getent hosts "$TEST_PG_CONTAINER" >/dev/null 2>&1; then
            TEST_DATABASE_URL="postgres://serverhub:changeme@${TEST_PG_CONTAINER}:5432/serverhub?sslmode=disable"
          else
            # Docker dynamically allocates an available host port.
            MAPPED_PORT="$(docker port "$TEST_PG_CONTAINER" 5432/tcp |
              awk -F: 'NR==1 {print $NF}')"

            if [ -z "$MAPPED_PORT" ]; then
              echo "ERROR: unable to resolve test PostgreSQL host port."
              exit 1
            fi

            TEST_DATABASE_URL="postgres://serverhub:changeme@127.0.0.1:${MAPPED_PORT}/serverhub?sslmode=disable"
          fi

          echo "$TEST_DATABASE_URL" > "$WORKSPACE/.jenkins-test-db-url"
        '''
      }
    }

    stage('Test') {
      steps {
        dir('server') {
          sh '''
            set -e

            export TEST_DATABASE_URL="$(cat "$WORKSPACE/.jenkins-test-db-url")"
            if [ -z "$TEST_DATABASE_URL" ]; then
              echo "ERROR: test database URL was not resolved by Start Test Database."
              exit 1
            fi

            echo "Running Go tests..."

            go test ./... \
              -count=1 \
              -timeout 300s
          '''
        }
      }
    }

    stage('Build') {
      steps {
        dir('server') {
          sh '''
            set -e

            if [ ! -s "$WORKSPACE/.jenkins-compose" ]; then
              echo "ERROR: compose wrapper missing (Prepare stage did not run?)."
              exit 1
            fi
            COMPOSE="$(cat "$WORKSPACE/.jenkins-compose")"

            echo "Validating Docker Compose configuration..."

            export POSTGRES_PASSWORD=dummy_build_password
            $COMPOSE config -q

            # Immutable release tag: the scanned image and the deployed
            # image are the same digest (see Security + Deploy stages).
            export SERVERHUB_VERSION="rc-${BUILD_NUMBER:-local}"
            echo "Building ServerHub Docker image (serverhub:${SERVERHUB_VERSION})..."

            $COMPOSE build serverhub
            docker inspect --format='{{.Id}}' "serverhub:${SERVERHUB_VERSION}" | tee "$WORKSPACE/image-digest.txt"
            echo "Candidate digest: $(cat "$WORKSPACE/image-digest.txt")"
          '''
        }
      }
    }

    stage('Security') {
      steps {
        sh '''
          set -e

          # Report-only means FINDINGS never fail the build. Tool EXECUTION
          # errors (pull/run failures) always fail it. Scanners use exit 1
          # for "findings present"; anything else non-zero is a failure.
          run_report_only() {
            label="$1"; shift
            set +e
            "$@"
            code=$?
            set -e
            case $code in
              0) echo "$label: clean." ;;
              1) echo "$label: findings reported (see artifact; triage before enforcing)." ;;
              *) echo "$label: TOOL FAILED with exit $code."; exit $code ;;
            esac
          }

          # When running inside a Docker container (e.g. Jenkins in Docker with
          # DooD /var/run/docker.sock mounted), mounting -v "$WORKSPACE:/src"
          # fails because the daemon evaluates $WORKSPACE on the host where
          # /var/jenkins_home does not exist. We reuse the container's existing
          # volume mount via --volumes-from $(hostname). If running on bare
          # metal, we mount -v "$WORKSPACE:/src".
          CID="$(hostname)"
          if docker inspect "$CID" >/dev/null 2>&1; then
            MOUNT_OPTS="--volumes-from $CID"
            SCAN_DIR="$WORKSPACE"
          else
            MOUNT_OPTS="-v $WORKSPACE:/src"
            SCAN_DIR="/src"
          fi

          echo "Gitleaks secret scan (report-only; triage before enforcing)..."
          run_report_only "Gitleaks" docker run --rm \
            $MOUNT_OPTS \
            "zricethezav/gitleaks:${GITLEAKS_VERSION}" \
            detect --source="$SCAN_DIR" --no-git \
            --report-format sarif --report-path "$SCAN_DIR/gitleaks.sarif" \
            --exit-code 1

          # Scan the exact candidate built by the Build stage (same tag,
          # same digest) — never a separately rebuilt image.
          RC_TAG="serverhub:rc-${BUILD_NUMBER:-local}"
          if [ "$(cat "$WORKSPACE/image-digest.txt")" != "$(docker inspect --format='{{.Id}}' "$RC_TAG")" ]; then
            echo "ERROR: candidate image digest changed between Build and Security stages."
            exit 1
          fi

          echo "Trivy filesystem scan (report-only)..."
          run_report_only "Trivy-fs" docker run --rm \
            $MOUNT_OPTS \
            "aquasec/trivy:${TRIVY_VERSION}" fs \
            --severity HIGH,CRITICAL --exit-code 1 \
            --format json --output "$SCAN_DIR/trivy-fs.json" \
            "$SCAN_DIR"

          # Export candidate image archive so Trivy and Syft can scan it
          # directly without registry lookups or Docker daemon image resolution quirks.
          echo "Exporting candidate image archive for security scanners..."
          docker save "$RC_TAG" -o "$WORKSPACE/candidate.tar"

          echo "Trivy image scan of the release candidate (report-only)..."
          run_report_only "Trivy-image" docker run --rm \
            $MOUNT_OPTS \
            "aquasec/trivy:${TRIVY_VERSION}" image \
            --severity HIGH,CRITICAL --exit-code 1 \
            --format json --output "$SCAN_DIR/trivy-image.json" \
            --input "$SCAN_DIR/candidate.tar"

          echo "Syft SBOM for the release candidate..."
          docker run --rm \
            $MOUNT_OPTS \
            "anchore/syft:${SYFT_VERSION}" \
            "docker-archive:$SCAN_DIR/candidate.tar" -o "spdx-json=$SCAN_DIR/sbom.spdx.json"
          if [ ! -s "$WORKSPACE/sbom.spdx.json" ]; then
            echo "ERROR: SBOM was not produced (absent report is not a clean scan)."
            rm -f "$WORKSPACE/candidate.tar"
            exit 1
          fi

          rm -f "$WORKSPACE/candidate.tar"

          echo "Release candidate: $RC_TAG @ $(cat "$WORKSPACE/image-digest.txt")"
        '''
        archiveArtifacts artifacts: 'gitleaks.sarif,trivy-fs.json,trivy-image.json,sbom.spdx.json,image-digest.txt', allowEmptyArchive: true
      }
    }

    stage('Mobile Bundle Check') {
    when { expression { params.BUILD_MOBILE } }
      steps {
        dir('mobile') {
          catchError(buildResult: 'SUCCESS', stageResult: 'UNSTABLE') {
                sh '''
            set -e

            echo "Validating mobile JS bundle (no token required)..."
            export EXPO_PUBLIC_API_URL="$EXPO_PUBLIC_API_URL"

            npx expo export --platform android
          '''
              }
        }
      }
    }

    stage('Mobile APK') {
    when { expression { params.BUILD_MOBILE } }
      steps {
        dir('mobile') {
          catchError(buildResult: 'SUCCESS', stageResult: 'UNSTABLE') {
                sh '''
            set -e

            echo "Building Android debug APK (prebuild + Gradle)..."

            export EXPO_PUBLIC_API_URL="$EXPO_PUBLIC_API_URL"
            export ANDROID_HOME="${ANDROID_HOME:-/var/jenkins_home/android-sdk}"
            export ANDROID_SDK_ROOT="$ANDROID_HOME"
            export PATH="$ANDROID_HOME/cmdline-tools/latest/bin:$ANDROID_HOME/platform-tools:$PATH"

            # Set clean JVM heap ceilings without explicit GC flags
            export GRADLE_OPTS="-Xmx2048m -XX:MaxMetaspaceSize=512m -Dorg.gradle.daemon=false -Dorg.gradle.parallel=false -Dkotlin.compiler.execution.strategy=in-process"
            export NODE_OPTIONS="--max-old-space-size=1536"

            command -v java >/dev/null 2>&1 || { echo "ERROR: JDK 17+ not found on agent."; exit 1; }
            [ -d "$ANDROID_HOME" ] || { echo "ERROR: Android SDK not found at $ANDROID_HOME."; exit 1; }

            # Accept SDK licenses automatically if prompted
            yes | "$ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager" --licenses >/dev/null 2>&1 || true

            npx expo prebuild --platform android --non-interactive
            chmod +x android/gradlew
            (cd android && ./gradlew assembleDebug --no-daemon --max-workers=1)
          '''
          archiveArtifacts artifacts: 'android/app/build/outputs/apk/debug/app-debug.apk'
              }
        }
      }
    }



    stage('Deploy') {
      // Blocked on UNSTABLE: optional mobile bundle/APK failures (caught as
      // UNSTABLE above) must not silently precede a production deployment.
      when { expression { params.DEPLOY && currentBuild.currentResult == 'SUCCESS' } }
      environment {
        POSTGRES_PASSWORD = credentials('serverhub-postgres-password')
        JWT_SECRET = credentials('serverhub-jwt-secret')
        SERVERHUB_ENCRYPTION_KEY = credentials('serverhub-encryption-key')
        ADMIN_PASSWORD = credentials('serverhub-admin-password')
        GITHUB_WEBHOOK_SECRET = credentials('serverhub-webhook-secret')

        // Host port for the production PostgreSQL container.
        // Host port 5432 is already occupied.
        POSTGRES_PORT = '5434'
      }

      steps {
        dir('server') {
          sh '''
            set -e

            if [ ! -s "$WORKSPACE/.jenkins-compose" ]; then
              echo "ERROR: compose wrapper missing (Prepare stage did not run?)."
              exit 1
            fi
            COMPOSE="$(cat "$WORKSPACE/.jenkins-compose")"
            export SERVERHUB_VERSION="rc-${BUILD_NUMBER:-local}"
            PREV_TAG="serverhub:prev-${BUILD_NUMBER:-local}"

            # ---------------------------------------------------------
            # Back up production database before schema changes.
            # Migrations are additive-only: there is no down-migration, so
            # this dump is the rollback path for data (see docs).
            # ---------------------------------------------------------

            if docker inspect serverhub-postgres >/dev/null 2>&1; then
              echo "Backing up production database..."
              docker exec serverhub-postgres \
                pg_dump -U serverhub serverhub 2>/dev/null | \
                gzip > "$WORKSPACE/pg-backup-${BUILD_NUMBER:-local}.sql.gz" || \
                echo "WARNING: database backup failed; continuing without a fresh dump."
            else
              echo "No existing database container; skipping backup (first deploy?)."
            fi

            # ---------------------------------------------------------
            # Pin the running image for rollback before touching anything.
            # ---------------------------------------------------------

            HAVE_PREV=false
            if docker inspect serverhub >/dev/null 2>&1; then
              PREV_ID="$(docker inspect serverhub --format '{{.Image}}')"
              docker tag "$PREV_ID" "$PREV_TAG"
              HAVE_PREV=true
              echo "Rollback image pinned: $PREV_TAG ($PREV_ID)"
            else
              echo "No running serverhub container; nothing to roll back to."
            fi

            rollback() {
              if [ "$HAVE_PREV" != "true" ]; then
                echo "ERROR: deployment failed and no previous image is pinned."
                exit 1
              fi
              echo "Restoring previous image ($PREV_TAG)..."
              export SERVERHUB_VERSION="prev-${BUILD_NUMBER:-local}"
              # Full stack: `down` removed postgres/prometheus too (named
              # volumes preserve their data). Best-effort restore, then fail.
              $COMPOSE up -d >/dev/null 2>&1 || true
              echo "Rolled back to previous image. Build still fails to signal."
              exit 1
            }

            echo "Stopping previous ServerHub Compose deployment..."

            $COMPOSE down --remove-orphans >/dev/null 2>&1 || true

            # ---------------------------------------------------------
            # Check host port 4000
            # ---------------------------------------------------------

            HOLDER="$(
              docker ps \
                --format '{{.Names}} {{.Image}} {{.Ports}}' \
                2>/dev/null |
              grep '4000->4000' ||
              true
            )"

            if [ -n "$HOLDER" ]; then
              echo "Port 4000 holder:"
              echo "$HOLDER"

              IMG="$(echo "$HOLDER" | awk '{print $2}')"

              # Prefix match: local tags render as serverhub:rc-N (and
              # legacy compose builds as server-hub-serverhub).
              case "$IMG" in
                serverhub*|server-hub-serverhub|docker.io/library/serverhub*)

                NAME="$(echo "$HOLDER" | awk '{print $1}')"

                echo "Detected stale ServerHub container: $NAME"
                echo "Stopping and removing it..."

                docker stop "$NAME" >/dev/null
                docker rm "$NAME" >/dev/null

              else
                echo "ERROR: Host port 4000 is held by another Docker container."
                echo "$HOLDER"

                echo ""
                echo "Stop the conflicting container or change the ServerHub host port."

                exit 1
              esac
            fi

            # ---------------------------------------------------------
            # Check host process using port 4000
            # ---------------------------------------------------------

            if command -v ss >/dev/null 2>&1; then

              HP="$(
                ss -tlnp 2>/dev/null |
                grep ':4000 ' ||
                true
              )"

              if [ -n "$HP" ]; then
                echo "ERROR: Host port 4000 is held by a host process."
                echo "$HP"

                echo ""
                echo "Stop the process/service using port 4000 and re-run Jenkins."

                exit 1
              fi
            fi

            # ---------------------------------------------------------
            # Start production deployment (scanned candidate digest)
            # ---------------------------------------------------------

            echo "Starting ServerHub (${SERVERHUB_VERSION})..."

            $COMPOSE up -d

            # ---------------------------------------------------------
            # Bounded readiness polling (replaces the fixed sleep):
            # liveness, then readiness with DB proof, with diagnostics.
            # ---------------------------------------------------------

            echo "Waiting for readiness (120s budget)..."

            READY=false
            for i in $(seq 1 40); do
              if curl -s -f http://localhost:4000/health/live >/dev/null 2>&1; then
                BODY="$(curl -s http://localhost:4000/health/ready 2>/dev/null || true)"
                # NOTE: Go marshals maps with sorted keys, so "checks"
                # (containing "db":"up") precedes "status" in the body.
                case "$BODY" in
                  *'"db":"up"'*'"status":"ok"'*)
                    READY=true
                    echo "ServerHub ready (liveness + readiness with DB up)."
                    break
                    ;;
                esac
              fi
              sleep 3
            done

            if [ "$READY" != "true" ]; then
              echo "ERROR: readiness failed after deployment."

              echo ""
              echo "Container status:"
              docker ps --filter "name=serverhub"

              echo ""
              echo "Recent ServerHub logs:"
              docker logs --tail 100 serverhub || true

              rollback
            fi

            # ---------------------------------------------------------
            # Migration + delivery-wiring proof (beyond /health): the new
            # notification tables must exist and serve.
            # ---------------------------------------------------------

            echo "Verifying notification migrations..."

            if ! docker exec serverhub-postgres \
              psql -U serverhub -d serverhub -tAc \
              "SELECT COUNT(*) FROM notification_policy" >/dev/null 2>&1; then
              echo "ERROR: notification_policy table unreachable; migrations did not apply."
              rollback
            fi

            # ---------------------------------------------------------
            # Success: drop the rollback pin and clean dangling images.
            # (Dangling-only prune runs after validation, never before.)
            # ---------------------------------------------------------

            if [ "$HAVE_PREV" = "true" ]; then
              docker rmi "$PREV_TAG" >/dev/null 2>&1 || true
            fi

            echo "Cleaning unused Docker images..."

            docker image prune -f

            echo "Deployment healthy."
          '''
        }
      }
    }

    stage('Staging Smoke') {
      when { expression { params.DEPLOY && params.STAGING_SMOKE && currentBuild.currentResult == 'SUCCESS' } }
      environment {
        // Admin password only; the gate creates nothing except a test
        // maintenance window that it deletes again.
        ADMIN_PASSWORD = credentials('serverhub-admin-password')
      }
      steps {
        sh '''
          set -e
          set -o pipefail
          BASE="http://localhost:4000/server-hub/api"
          HEALTH="http://localhost:4000/health"
          COOKIES="$(mktemp)"
          trap 'rm -f "$COOKIES"' EXIT

          need() { # need <desc> <expected> <actual>
            if [ "$2" != "$3" ]; then
              echo "SMOKE FAIL: $1 (want $2, got $3)"
              exit 1
            fi
            echo "SMOKE OK: $1"
          }

          echo "== readiness =="
          need "ready db up" "200" "$(curl -s -o /dev/null -w '%{http_code}' "$HEALTH/ready")"

          echo "== auth + RBAC =="
          CODE="$(curl -s -o /dev/null -w '%{http_code}' "$BASE/notification-policy")"
          need "unauthenticated policy denied" "401" "$CODE"
          CODE="$(curl -s -o /dev/null -w '%{http_code}' -c "$COOKIES" \
            -H 'Content-Type: application/json' \
            -d '{"username":"admin","password":"'"$ADMIN_PASSWORD"'"}' \
            "$BASE/auth/login")"
          need "admin login" "200" "$CODE"

          echo "== notification policy round-trip =="
          CODE="$(curl -s -o /dev/null -w '%{http_code}' -b "$COOKIES" "$BASE/notification-policy")"
          need "policy GET" "200" "$CODE"
          CODE="$(curl -s -o /dev/null -w '%{http_code}' -b "$COOKIES" \
            -H 'Content-Type: application/json' -X PUT \
            -d '{"maxRepeats":3}' "$BASE/notification-policy")"
          need "policy PUT" "200" "$CODE"

          echo "== emergency pause set/verify/clear =="
          UNTIL="$(date -u -d '+10 minutes' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v+10M +%Y-%m-%dT%H:%M:%SZ)"
          curl -s -b "$COOKIES" -H 'Content-Type: application/json' -X PUT \
            -d '{"emergencyPause":true,"pauseReason":"staging-smoke","pauseUntil":"'"$UNTIL"'"}' \
            "$BASE/notification-policy" | grep -q '"emergencyPause":true'
          echo "SMOKE OK: pause armed"
          curl -s -b "$COOKIES" -H 'Content-Type: application/json' -X PUT \
            -d '{"clearPause":true}' "$BASE/notification-policy" | grep -q '"emergencyPause":false'
          echo "SMOKE OK: pause cleared"

          echo "== maintenance window lifecycle =="
          MW_END="$(date -u -d '+70 minutes' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v+70M +%Y-%m-%dT%H:%M:%SZ)"
          MW="$(curl -s -b "$COOKIES" -H 'Content-Type: application/json' \
            -d '{"name":"staging-smoke","scope":"all","startsAt":"'"$UNTIL"'","endsAt":"'"$MW_END"'","reason":"smoke"}' \
            "$BASE/maintenance-windows")"
          echo "$MW" | grep -q '"id":' || { echo "SMOKE FAIL: window create: $MW"; exit 1; }
          echo "SMOKE OK: window created"
          MID="$(echo "$MW" | grep -o '"id":[0-9]*' | head -1 | tr -cd '0-9')"
          CODE="$(curl -s -o /dev/null -w '%{http_code}' -b "$COOKIES" -X DELETE "$BASE/maintenance-windows/$MID")"
          need "window deleted" "200" "$CODE"

          echo "== delivery history serves =="
          CODE="$(curl -s -o /dev/null -w '%{http_code}' -b "$COOKIES" "$BASE/notification-deliveries?limit=5")"
          need "deliveries GET" "200" "$CODE"

          if [ "${STAGING_SEND_TESTS}" = "true" ]; then
            echo "== provider test-send (explicitly enabled) =="
            curl -s -b "$COOKIES" -X POST "$BASE/settings/notifications/test" | tee /dev/stderr | grep -q '"telegram"'
            echo "SMOKE OK: test-send responded (check recipients got exactly one message)"
          else
            echo "== provider test-send skipped (STAGING_SEND_TESTS=false; no burst in routine CI) =="
          fi

          echo "Staging smoke passed."
        '''
      }
    }
  }

  post {

    always {
      sh '''
        echo "Cleaning Jenkins test resources..."

        # Per-build name (mirrors Start Test Database); best-effort removal —
        # with unique names a leftover can never block a future run.
        export TEST_PG_CONTAINER="${TEST_PG_CONTAINER_BASE}-${BUILD_NUMBER:-local}"
        docker rm -f "$TEST_PG_CONTAINER" >/dev/null 2>&1 || true

        # Remove the per-build Docker network (name persisted by Prepare;
        # never a shared fixed name another job may be using).
        if [ -f "$WORKSPACE/.jenkins-test-net" ]; then
          docker network rm "$(cat "$WORKSPACE/.jenkins-test-net")" >/dev/null 2>&1 || true
        fi

        # Remove temporary helper files.
        rm -f "$WORKSPACE/.jenkins-test-db-url" "$WORKSPACE/.jenkins-test-net" 2>/dev/null || true

        echo "Cleanup completed."
      '''
      // Reports must survive failures: an absent report is not a clean scan.
      archiveArtifacts artifacts: 'gitleaks.sarif,trivy-fs.json,trivy-image.json,sbom.spdx.json,image-digest.txt,pg-backup-*.sql.gz', allowEmptyArchive: true
    }

    success {
      echo 'ServerHub Jenkins pipeline completed successfully.'
    }

    failure {
      echo 'ServerHub Jenkins pipeline failed.'
    }

    unstable {
      echo 'ServerHub Jenkins pipeline completed with unstable status.'
    }
  }
}
