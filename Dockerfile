# Build stage
FROM --platform=${BUILDPLATFORM} golang:1.24 AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o karpenter-do ./cmd/controller/

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /
COPY --from=builder /workspace/karpenter-do .
USER 65532:65532

ENTRYPOINT ["/karpenter-do"]
