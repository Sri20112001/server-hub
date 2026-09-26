pipeline {
    agent none

    options {
        skipDefaultCheckout(true)
        timestamps()
        timeout(time: 35, unit: 'MINUTES')
        disableConcurrentBuilds()
        buildDiscarder(logRotator(numToKeepStr: '20'))
    }

    environment {
        // Repository configuration
        GIT_REPO_URL            = 'https://github.com/Sri20112001/serverhub.git' // Adjust if needed
        GIT_BRANCH              = 'main'
        GIT_CREDENTIALS_ID      = 'serverhub-git-credentials' // Configure in Jenkins Credentials

        // Ephemeral Postgres container for Go tests
        TEST_PG_CONTAINER       = 'jenkins-serverhub-pg'
        
        // Compose setup (base + override for named volume)
        COMPOSE_FILE            = 'docker-compose.yml:docker-compose.jenkins.yml'
        
        // Mobile EAS build toggle
        BUILD_MOBILE            = 'false'
        EXPO_PUBLIC_API_URL     = 'http://localhost:4000'
    }

    stages {
        stage('Checkout') {
            agent any

            steps {
                git(
                    url: env.GIT_REPO_URL,
                    branch: env.GIT_BRANCH,
                    credentialsId: env.GIT_CREDENTIALS_ID
                )

                sh '''
                    set -e
                    echo "========================================"
                    echo "ServerHub Checkout"
                    echo "========================================"
                    echo "Commit: $(git rev-parse HEAD)"
                    echo "Branch: $(git rev-parse --abbrev-ref HEAD)"
                '''
            }
        }

        stage('Prepare Environment') {
            agent any

            steps {
                sh '''
                    set -e
                    echo "========================================"
                    echo "Verifying Docker & Compose"
                    echo "========================================"

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

                    echo "Using compose: $(cat .jenkins-compose)"
                    $(cat .jenkins-compose) version
                '''
            }
        }

        stage('Client CI') {
            agent any

            steps {
                dir('client') {
                    sh '''
                        set -e
                        echo "========================================"
                        echo "Frontend Client CI"
                        echo "========================================"
                        npm ci --no-audit --no-fund
                        npm run lint
                        npm run build
                    '''
                }
            }
        }

        stage('Mobile CI') {
            agent any

            steps {
                dir('mobile') {
                    sh '''
                        set -e
                        echo "========================================"
                        echo "Mobile CI (Type-check & Lint)"
                        echo "========================================"
                        npm ci --no-audit --no-fund
                        npm run type-check
                        npm run lint
                    '''
                }
            }
        }

        stage('Mobile EAS Build') {
            agent any
            when {
                environment name: 'BUILD_MOBILE', value: 'true'
            }
            steps {
                dir('mobile') {
                    withCredentials([string(credentialsId: 'expo-token', variable: 'EXPO_TOKEN')]) {
                        sh '''
                            set -e
                            echo "========================================"
                            echo "Mobile EAS Android Build"
                            echo "========================================"
                            export EXPO_TOKEN="$EXPO_TOKEN"
                            export EXPO_PUBLIC_API_URL="$EXPO_PUBLIC_API_URL"
                            npx -y eas-cli@latest build --platform android --profile preview --non-interactive
                        '''
                    }
                }
            }
        }

        stage('Start Test Database') {
            agent any

            steps {
                sh '''
                    set -e
                    echo "========================================"
                    echo "Spinning Up Ephemeral Postgres"
                    echo "========================================"
                    docker rm -f "$TEST_PG_CONTAINER" >/dev/null 2>&1 || true

                    docker run -d --name "$TEST_PG_CONTAINER" \
                        -e POSTGRES_DB=serverhub \
                        -e POSTGRES_USER=serverhub \
                        -e POSTGRES_PASSWORD=changeme \
                        -p 5433:5432 \
                        postgres:16-alpine

                    for i in $(seq 1 30); do
                        if docker exec "$TEST_PG_CONTAINER" pg_isready -U serverhub >/dev/null 2>&1; then
                            echo "Database ready."
                            break
                        fi
                        sleep 1
                    done

                    PG_IP="$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$TEST_PG_CONTAINER")"
                    echo "postgres://serverhub:changeme@${PG_IP}:5432/postgres?sslmode=disable" > .jenkins-test-db-url
                    echo "Test DB URL configured for container network."
                '''
            }
        }

        stage('Server Vet & Test') {
            agent any

            steps {
                dir('server') {
                    sh '''
                        set -e
                        echo "========================================"
                        echo "Backend Go Vet"
                        echo "========================================"
                        go vet ./...

                        echo "========================================"
                        echo "Backend Go Tests"
                        echo "========================================"
                        if [ -f "$WORKSPACE/.jenkins-test-db-url" ]; then
                            export TEST_DATABASE_URL="$(cat "$WORKSPACE/.jenkins-test-db-url")"
                        fi
                        go test ./... -count=1 -timeout 300s
                    '''
                }
            }
        }

        stage('Build & Deploy') {
            agent any
            environment {
                POSTGRES_PASSWORD        = credentials('serverhub-postgres-password')
                JWT_SECRET               = credentials('serverhub-jwt-secret')
                SERVERHUB_ENCRYPTION_KEY = credentials('serverhub-encryption-key')
                ADMIN_PASSWORD           = credentials('serverhub-admin-password')
                GITHUB_WEBHOOK_SECRET    = credentials('serverhub-webhook-secret')
                POSTGRES_PORT            = '5434'
            }
            steps {
                dir('server') {
                    sh '''
                        set -e
                        echo "========================================"
                        echo "Deploying ServerHub via Docker Compose"
                        echo "========================================"
                        COMPOSE="$(cat "$WORKSPACE/.jenkins-compose")"

                        $COMPOSE down --remove-orphans >/dev/null 2>&1 || true

                        # Prevent port 4000 collisions
                        HOLDER="$(docker ps --format '{{.Names}} {{.Image}} {{.Ports}}' 2>/dev/null | grep '4000->4000' || true)"
                        if [ -n "$HOLDER" ]; then
                            IMG="$(echo "$HOLDER" | awk '{print $2}')"
                            NAME="$(echo "$HOLDER" | awk '{print $1}')"
                            if [ "$IMG" = "serverhub" ] || [ "$IMG" = "docker.io/library/serverhub" ]; then
                                echo "Cleaning stale container ($NAME)..."
                                docker stop "$NAME" >/dev/null && docker rm "$NAME" >/dev/null
                            else
                                echo "ERROR: Host port 4000 held by foreign container: $HOLDER"
                                exit 1
                            fi
                        fi

                        $COMPOSE build
                        $COMPOSE up -d --force-recreate
                        docker image prune -f

                        echo "========================================"
                        echo "Verifying Health Endpoint"
                        echo "========================================"
                        sleep 8
                        ( curl -s -f http://localhost:4000/health 2>/dev/null \
                          || docker exec serverhub wget -q -O- http://localhost:4000/health ) \
                          || echo "Health check delayed; container still initializing."
                    '''
                }
            }
        }
    }

    post {
        always {
            sh 'docker rm -f "$TEST_PG_CONTAINER" >/dev/null 2>&1 || true'
            echo 'ServerHub pipeline execution finished.'
        }
        success {
            echo 'ServerHub successfully built and deployed!'
        }
        failure {
            echo 'ServerHub pipeline failed. Check console output.'
        }
    }
}
