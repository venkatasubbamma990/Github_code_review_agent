# ---- Build stage ----
FROM golang:1.26-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/codereviewagent ./cmd/server

# Install gosec into the module cache bin path.
RUN go install github.com/securego/gosec/v2/cmd/gosec@v2.22.3

# ---- Run stage ----
# Python base so semgrep is available; Go toolchain copied for gosec package analysis.
FROM python:3.12-alpine

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata wget git \
    && pip install --no-cache-dir --break-system-packages semgrep \
    && addgroup -S appgroup \
    && adduser -S -h /home/appuser -G appgroup appuser \
    && mkdir -p /home/appuser/.semgrep /tmp/go /tmp/gocache \
    && chown -R appuser:appgroup /home/appuser /tmp/go /tmp/gocache

# Go toolchain (needed by gosec when scanning Go packages)
COPY --from=builder /usr/local/go /usr/local/go
COPY --from=builder /go/bin/gosec /usr/local/bin/gosec
COPY --from=builder /app/codereviewagent /app/codereviewagent

ENV PATH="/usr/local/go/bin:/usr/local/bin:${PATH}" \
    GOPATH=/tmp/go \
    GOCACHE=/tmp/gocache \
    HOME=/home/appuser \
    GOSEC_PATH=gosec \
    SEMGREP_PATH=semgrep

USER appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/codereviewagent"]
