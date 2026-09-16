# Red Hat Provisions — GRPCRoute Demo

A demo application for a fictitious doughnut company that demonstrates how **GRPCRoute** in the [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/) routes gRPC traffic from external clients.

## Architecture

```
                     ┌──────────────────────────────────────────────┐
                     │              Kubernetes Cluster              │
                     │                                              │
Browser ──────────→  Gateway (Envoy / Istio)                        │
  │                  │  │                                           │
  │ HTTP (page)      │  ├── [HTTPRoute]  ──→  Frontend (nginx)      │
  │                  │  │                     static HTML/JS        │
  │ gRPC-Web (API)   │  │                                          │
  │                  │  └── [GRPCRoute] ──→  Backend (connect-go)   │
  │                  │       matches:         DoughnutService       │
  │                  │       DoughnutService                        │
  │                  │       /ListFlavors                           │
  │                  │       /OrderDoughnuts                        │
                     └──────────────────────────────────────────────┘
```

The browser loads the page via **HTTPRoute**, then makes **gRPC-Web** calls directly through the **GRPCRoute** — demonstrating Gateway API's native gRPC routing exposed externally.

Istio's Envoy automatically translates gRPC-Web → native gRPC before routing, so the GRPCRoute matches on service and method names as expected.

## Prerequisites

- Go 1.23+
- Docker
- `kubectl` configured for an OpenShift cluster with Istio / Gateway API
- Gateway API CRDs installed
- `protoc` + Go plugins (only if modifying the proto)

## Quick Start (Local)

The backend uses [ConnectRPC](https://connectrpc.com/) which speaks native gRPC, gRPC-Web, and Connect on the same port — so local dev works without a proxy:

```bash
# Terminal 1: start the backend
go run ./backend

# Terminal 2: serve the frontend
cd frontend/static && python3 -m http.server 8080

# Open http://localhost:8080
```

Or use the Makefile:

```bash
make run-local
```

## Quick Start (Kubernetes)

1. Edit `k8s/gateway.yaml` and set `gatewayClassName` to match your cluster
2. Edit image references in `k8s/backend.yaml` and `k8s/frontend.yaml`
3. Build and push:

```bash
make IMAGE_REGISTRY=your-registry.example.com push
```

4. Deploy:

```bash
make deploy
```

5. Get the Gateway address:

```bash
kubectl -n red-hat-provisions get gateway doughnut-gateway
```

6. Open the Gateway address in your browser. The page loads via HTTPRoute; doughnut orders flow via GRPCRoute.

## Testing GRPCRoute with grpcurl

You can also test the GRPCRoute directly from the command line:

```bash
GATEWAY_URL=$(kubectl -n red-hat-provisions get gateway doughnut-gateway \
  -o jsonpath='{.status.addresses[0].value}')

# List flavors
grpcurl -plaintext $GATEWAY_URL:80 doughnut.v1.DoughnutService/ListFlavors

# Place an order
grpcurl -plaintext -d '{"customer_name":"Homer","items":[{"flavor":"Glazed","quantity":6}]}' \
  $GATEWAY_URL:80 doughnut.v1.DoughnutService/OrderDoughnuts
```

## GRPCRoute Configuration

The `GRPCRoute` in `k8s/grpcroute.yaml` matches gRPC traffic by service and method name:

```yaml
spec:
  parentRefs:
    - name: doughnut-gateway
      sectionName: http      # same listener as HTTPRoute
  rules:
    - matches:
        - method:
            service: doughnut.v1.DoughnutService
            method: ListFlavors
        - method:
            service: doughnut.v1.DoughnutService
            method: OrderDoughnuts
      backendRefs:
        - name: doughnut-backend
          port: 50051
```

Both HTTPRoute and GRPCRoute attach to the same Gateway listener. The Gateway distinguishes traffic by content type — `application/grpc-web` is handled by the gRPC-Web filter and matched by GRPCRoute; regular HTTP requests are matched by HTTPRoute.

## Project Structure

```
grpc-demo/
├── proto/doughnut/v1/doughnut.proto   # gRPC service definition
├── gen/                                # generated Go code (protoc + connect-go)
├── backend/                            # gRPC server (connect-go)
│   ├── main.go
│   └── Dockerfile
├── frontend/                           # static web UI
│   ├── static/
│   │   ├── index.html                  # page + grpc-web client (vanilla JS)
│   │   └── doughnut.proto              # proto for protobufjs runtime loading
│   ├── nginx.conf
│   └── Dockerfile
├── k8s/                                # Kubernetes manifests
│   ├── gateway.yaml                    # Gateway resource
│   ├── grpcroute.yaml                  # GRPCRoute (external gRPC)
│   ├── httproute.yaml                  # HTTPRoute (static files)
│   ├── backend.yaml                    # Backend Deployment + Service
│   └── frontend.yaml                   # Frontend Deployment + Service
├── go.mod
└── Makefile
```

## Modifying the Proto

```bash
# Install plugins (one-time)
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest

# Regenerate
make proto
```
