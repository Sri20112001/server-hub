
# 1. Build the React frontend with the shared package available
FROM node:22-alpine AS frontend-build

WORKDIR /workspace

# Install frontend dependencies from the existing lockfile
COPY client/package*.json ./client/
WORKDIR /workspace/client
RUN npm ci

# Preserve the relative paths expected by Vite and TypeScript:
# /workspace/client/../packages/shared
WORKDIR /workspace
COPY client/ ./client/
COPY packages/shared/ ./packages/shared/

WORKDIR /workspace/client
ARG VITE_API_URL=""
ENV VITE_API_URL=$VITE_API_URL
RUN npm run build


# 2. Build the Go backend
FROM golang:1.25-alpine AS backend-build

WORKDIR /app
COPY server/go.mod server/go.sum ./
RUN go mod download

COPY server/ ./
RUN CGO_ENABLED=0 go build -o /serverhub ./cmd/server


# 3. Final runtime image
FROM alpine:3.20

# curl is the in-container HTTP client for CI health/smoke checks: the
# Jenkins agent is itself containerized, so host-published loopback ports
# are unreachable from it and probes must run inside the app container.
RUN apk add --no-cache ca-certificates curl docker-cli && \
    adduser -D -u 1001 serverhub

WORKDIR /app

COPY --from=backend-build /serverhub /app/serverhub
COPY --from=frontend-build /workspace/client/dist /app/client/dist

RUN chown -R serverhub:serverhub /app

VOLUME ["/app/data"]
EXPOSE 4000

ENV PORT=4000 FRONTEND_DIR=/app/client/dist

USER serverhub
CMD ["/app/serverhub"]