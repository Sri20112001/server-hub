# 1. Build the React Frontend
FROM node:20-alpine AS frontend-build
WORKDIR /app
COPY client/package*.json ./
RUN npm ci
COPY client/ .
ARG VITE_API_URL=/server-hub/api
ENV VITE_API_URL=$VITE_API_URL
RUN npm run build

# 2. Build the Go Backend
FROM golang:1.26-alpine AS backend-build
WORKDIR /app
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ .
RUN CGO_ENABLED=0 go build -o /serverhub ./cmd/server

# 3. Final Image
FROM alpine:3.20
RUN apk add --no-cache ca-certificates docker-cli
WORKDIR /app
COPY --from=backend-build /serverhub /app/serverhub
COPY --from=frontend-build /app/dist /app/client/dist
VOLUME ["/app/data"]
EXPOSE 4000
ENV PORT=4000 DB_PATH=/app/data/serverhub.db FRONTEND_DIR=/app/client/dist
CMD ["/app/serverhub"]
