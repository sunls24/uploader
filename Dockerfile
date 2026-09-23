# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM oven/bun:alpine AS bun-builder
WORKDIR /app
ENV ASTRO_TELEMETRY_DISABLED=1

COPY ./web/package.json ./web/bun.lock ./
RUN bun install --frozen-lockfile
COPY ./web .
RUN bun run build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
WORKDIR /app
ARG TARGETOS
ARG TARGETARCH
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=bun-builder /app/dist ./web/dist/
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags '-s -w' -o uploader cmd/main.go

FROM alpine AS runner
WORKDIR /app
RUN addgroup -S uploader && adduser -S -G uploader uploader \
    && mkdir /data && chown uploader:uploader /data
COPY --from=builder /app/uploader .

ENV HOST=0.0.0.0 \
    PORT=3000 \
    STORAGE_DIR=/data
VOLUME ["/data"]
EXPOSE 3000
USER uploader
CMD ["/app/uploader"]
