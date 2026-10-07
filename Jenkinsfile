// ServerHub CI/CD — Jenkins declarative pipeline.
// Agent prerequisites: Docker + Compose, Go >= 1.26, Node 22 LTS,
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
  }

  environment {
    // Base name for the throwaway test Postgres. Stages derive a per-build
    // container name ("${base}-${BUILD_NUMBER}") so a wedged container from
    // an older run can never collide with the current one.
    TEST_PG_CONTAINER_BASE = 'jenkins-serverhub-pg'
    TEST_PG_NETWORK = 'jenkins-test-network'

    COMPOSE_FILE = 'docker-compose.yml:docker-compose.jenkins.yml'

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

      echo "Checking Docker..."
      docker --version

      echo "Checking Docker Compose..."
      docker compose version

      echo "Preparing test Docker network..."
      docker network create "$TEST_PG_NETWORK" 2>/dev/null || true

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
              catchError(buildResult: 'SUCCESS', stageResult: 'UNSTABLE') {
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
          docker rm -f "$TEST_PG_CONTAINER" >/dev/null 2>&1 || true

          # Make sure the network exists.
          docker network create "$TEST_PG_NETWORK" 2>/dev/null || true

          docker run -d \
            --name "$TEST_PG_CONTAINER" \
            --network "$TEST_PG_NETWORK" \
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

          echo "Test database: serverhub"
          echo "Test PostgreSQL container: $TEST_PG_CONTAINER"
          echo "Test Docker network: $TEST_PG_NETWORK"
        '''
      }
    }

    stage('Test') {
      steps {
        dir('server') {
          sh '''
            set -e

            export TEST_PG_CONTAINER="${TEST_PG_CONTAINER_BASE}-${BUILD_NUMBER:-local}"
            export TEST_DATABASE_URL="postgres://serverhub:changeme@${TEST_PG_CONTAINER}:5432/serverhub?sslmode=disable"

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

            COMPOSE="$(cat "$WORKSPACE/.jenkins-compose")"

            echo "Building ServerHub Docker images..."

            export POSTGRES_PASSWORD=dummy_build_password
            $COMPOSE build
          '''
        }
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
      when { expression { params.DEPLOY } }
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

            COMPOSE="$(cat "$WORKSPACE/.jenkins-compose")"

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

              if [ "$IMG" = "serverhub" ] || \
                 [ "$IMG" = "docker.io/library/serverhub" ]; then

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
              fi
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
            # Start production deployment
            # ---------------------------------------------------------

            echo "Starting ServerHub..."

            $COMPOSE up -d

            # ---------------------------------------------------------
            # Remove dangling images
            # ---------------------------------------------------------

            echo "Cleaning unused Docker images..."

            docker image prune -f

            # ---------------------------------------------------------
            # Wait for application startup
            # ---------------------------------------------------------

            echo "Waiting for ServerHub to start..."

            sleep 8

            # ---------------------------------------------------------
            # Health check
            # ---------------------------------------------------------

            echo "Running health check..."

            HEALTH_OK=false

            if curl -s -f \
              http://localhost:4000/health \
              >/dev/null 2>&1; then

              echo "ServerHub health check passed."
              HEALTH_OK=true

            elif docker exec serverhub \
              wget -q \
              -O- \
              http://localhost:4000/health \
              >/dev/null 2>&1; then

              echo "ServerHub health check passed from inside container."
              HEALTH_OK=true

            fi

            if [ "$HEALTH_OK" != "true" ]; then
              echo "ERROR: Health check failed after deployment."

              echo ""
              echo "Container status:"
              docker ps --filter "name=serverhub"

              echo ""
              echo "Recent ServerHub logs:"
              docker logs --tail 100 serverhub || true

              exit 1
            fi
          '''
        }
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

        # Remove temporary Docker network.
        docker network rm "$TEST_PG_NETWORK" >/dev/null 2>&1 || true

        # Remove temporary compose helper files.
        rm -f "$WORKSPACE/.jenkins-test-db-url" 2>/dev/null || true

        echo "Cleanup completed."
      '''
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
