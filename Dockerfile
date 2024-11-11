# ============= Compilation Stage ================
FROM golang:1.20-bullseye AS builder

WORKDIR /build
# Copy and download metal dependencies using go mod
COPY go.mod .
COPY go.sum .
RUN go mod download
# Copy the code into the container
COPY . .
# Build metalgo
RUN ./scripts/build.sh

# ============= Cleanup Stage ================
FROM debian:11-slim
WORKDIR /

# Copy the executables into the container
COPY --from=builder /build/bin/metal .
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
ENTRYPOINT [ "./metal" ]
