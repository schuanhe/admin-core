# syntax=docker/dockerfile:1
FROM node:24-alpine AS frontend
WORKDIR /build/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27.3-alpine AS backend
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/controller/ ./cmd/controller/
COPY internal/ ./internal/
COPY web/contract.go ./web/contract.go
COPY web/src/dynamic/catalog.json ./web/src/dynamic/catalog.json
COPY web/src/dynamic/example.json ./web/src/dynamic/example.json
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/controller ./cmd/controller \
    && mkdir -p /out/data

# 可选CI测试目标；独立于最终运行镜像，便于没有本机C编译器时执行竞态检测。
FROM backend AS backend-test
RUN apk add --no-cache gcc musl-dev \
    && CGO_ENABLED=1 go test -race ./internal/...

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=backend --chown=65532:65532 /out/controller /app/controller
COPY --from=backend --chown=65532:65532 /out/data /app/data
COPY --from=frontend --chown=65532:65532 /build/web/dist /app/web/dist
ENV CONTROLLER_LISTEN=0.0.0.0:8080 \
    CONTROLLER_DATABASE_PATH=/app/data/controller.db \
    CONTROLLER_STATIC_DIR=/app/web/dist
EXPOSE 8080
VOLUME ["/app/data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/app/controller", "-healthcheck"]
ENTRYPOINT ["/app/controller"]

