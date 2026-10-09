# syntax=docker/dockerfile:1

# Stage 1 — build the React frontend
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# Stage 2 — static Go binary with the frontend embedded
FROM golang:1.27.1-alpine AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY web/embed.go web/embed.go
COPY --from=web /src/web/dist web/dist
# pure-Go SQLite + static build => no libc required in the runtime image
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server-manager ./cmd/server

# Stage 3 — runtime: binary + CA certs, nothing else
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=server /out/server-manager /usr/local/bin/server-manager
ENV APP_PORT=8080 \
    APP_DATA_DIR=/data \
    APP_DATABASE=/data/app.db \
    APP_ENV=prod
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/server-manager"]
