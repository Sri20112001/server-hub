# ServerHub API
FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 go build -o /serverhub ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates docker-cli
WORKDIR /app
COPY --from=build /serverhub /app/serverhub
VOLUME ["/app/data"]
EXPOSE 4000
ENV PORT=4000 DB_PATH=/app/data/serverhub.db
CMD ["/app/serverhub"]
