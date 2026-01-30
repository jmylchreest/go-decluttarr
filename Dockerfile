# Build stage
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w \
        -X github.com/jmylchreest/go-decluttarr/internal/version.Version=${VERSION} \
        -X github.com/jmylchreest/go-decluttarr/internal/version.Commit=${COMMIT} \
        -X github.com/jmylchreest/go-decluttarr/internal/version.BuildDate=${BUILD_DATE}" \
    -o go-decluttarr ./cmd/go-decluttarr

# Runtime stage - minimal scratch image
FROM scratch

# Copy CA certificates for HTTPS
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy binary
COPY --from=builder /app/go-decluttarr /go-decluttarr

USER 65534:65534

ENTRYPOINT ["/go-decluttarr"]
