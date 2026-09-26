pipeline {
    agent none

    options {
        skipDefaultCheckout(true)
        timestamps()
    }

    environment {
        SERVER_PATH = '/home/administrator/homelabs-upload'
    }

    stages {
        stage('Checkout') {
            agent any

            steps {
                git(
                    url: 'https://github.com/Sri20112001/homelabs-upload.git',
                    branch: 'main',
                    credentialsId: 'github-homelabs-upload'
                )

                sh '''
                    set -e
                    echo "========================================"
                    echo "Checkout"
                    echo "========================================"
                    echo "Built commit:"
                    git rev-parse HEAD
                '''
            }
        }

        stage('Backend vet + test') {
            agent any

            steps {
                sh '''
                    set -e
                    cd server

                    echo "========================================"
                    echo "Go vet"
                    echo "========================================"
                    go vet ./...

                    echo "========================================"
                    echo "Go tests"
                    echo "========================================"
                    go test ./...

                    echo "Backend checks passed."
                '''
            }
        }

        stage('Frontend build') {
            agent any

            steps {
                sh '''
                    set -e
                    cd client

                    echo "========================================"
                    echo "Installing frontend dependencies"
                    echo "========================================"
                    npm ci

                    echo "========================================"
                    echo "Building frontend"
                    echo "========================================"
                    npm run build

                    echo "Frontend build completed."
                '''
            }
        }

        stage('Sync application to server') {
            agent any

            steps {
                withCredentials([
                    sshUserPrivateKey(
                        credentialsId: 'homelabs-ssh-key',
                        keyFileVariable: 'SSH_KEY',
                        usernameVariable: 'SSH_USER'
                    )
                ]) {
                    sh '''
                        set -e
                        echo "========================================"
                        echo "Deployment target"
                        echo "========================================"
                        echo "Host: $DEPLOY_HOST"
                        echo "User: $SSH_USER"
                        echo "Path: $SERVER_PATH"

                        echo "========================================"
                        echo "Creating deployment directory"
                        echo "========================================"
                        ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "$SSH_USER@$DEPLOY_HOST" "mkdir -p '$SERVER_PATH'"

                        echo "========================================"
                        echo "Syncing application"
                        echo "========================================"
                        export RSYNC_RSH="ssh -i '$SSH_KEY' -o StrictHostKeyChecking=no"

                        rsync -az --delete \
                            --exclude='.git/' \
                            --exclude='server/data/' \
                            ./ \
                            "$SSH_USER@$DEPLOY_HOST:$SERVER_PATH/"

                        echo "Application sync completed."
                    '''
                }
            }
        }

        stage('Deploy') {
            agent any

            steps {
                withCredentials([
                    sshUserPrivateKey(
                        credentialsId: 'homelabs-ssh-key',
                        keyFileVariable: 'SSH_KEY',
                        usernameVariable: 'SSH_USER'
                    )
                ]) {
                    sh '''
                        set -e
                        echo "========================================"
                        echo "Connecting to deployment server"
                        echo "========================================"

                        ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "$SSH_USER@$DEPLOY_HOST" bash -s << 'EOF'
                            set -e
                            cd "$SERVER_PATH/server"

                            echo "========================================"
                            echo "Stopping existing application"
                            echo "========================================"
                            docker compose down --remove-orphans || true

                            echo "========================================"
                            echo "Building Docker images"
                            echo "========================================"
                            docker compose build

                            echo "========================================"
                            echo "Starting application"
                            echo "========================================"
                            docker compose up -d --force-recreate

                            echo "========================================"
                            echo "Application containers"
                            echo "========================================"
                            docker compose ps

                            echo "========================================"
                            echo "Removing unused Docker images"
                            echo "========================================"
                            docker image prune -f

                            echo "========================================"
                            echo "Waiting for application"
                            echo "========================================"
                            sleep 8

                            echo "========================================"
                            echo "Checking health endpoint"
                            echo "========================================"
                            curl -f http://localhost:8080/health
                            echo ""
                            echo "Health check passed."

                            echo "========================================"
                            echo "Checking NodeVault root"
                            echo "========================================"
                            curl -fsS http://localhost:8888/ | grep -q NodeVault
                            echo "NodeVault root check passed."

                            echo "========================================"
                            echo "Checking NodeVault application"
                            echo "========================================"
                            curl -fsS http://localhost:8888/nodevault/ | grep -q NodeVault
                            echo "NodeVault application check passed."

                            echo "========================================"
                            echo "Deployment successful."
                            echo "========================================"
EOF
                    '''
                }
            }
        }
    }

    post {
        success {
            echo 'Pipeline succeeded.'
        }
        failure {
            echo 'Pipeline failed — check the failing stage log above.'
        }
        always {
            echo 'Pipeline execution completed.'
        }
    }
}
