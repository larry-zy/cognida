# gRPC Development Guide

Guide for implementing gRPC services with Python server and Go client, including proto file development.

## Usage

```
/grpc-dev [task]
```

**Tasks:**
- `proto` - Create or modify proto files
- `server` - Implement Python gRPC server
- `client` - Implement Go gRPC client
- `test` - Add integration tests
- `fix` - Fix common gRPC issues

## Key Principles

### 1. Proto File Development

- Use `syntax = "proto3"` for modern gRPC
- Add `option go_package` for Go code generation
- Group related messages and RPCs with comments
- Use `oneof` for mutually exclusive fields

### 2. Python Server (grpcio)

**Important:** Use **sync server** on Windows, async server has bugs:
```python
import grpc
from concurrent import futures

# Sync server with asyncio.run() wrapper for async functions
server = grpc.server(
    futures.ThreadPoolExecutor(max_workers=10),
    options=[
        ('grpc.max_receive_message_length', 100 * 1024 * 1024),
        ('grpc.max_send_message_length', 100 * 1024 * 1024),
    ]
)
```

**Sync methods with async wrapper:**
```python
def MethodName(self, request, context):
    return asyncio.run(self._method_async(request))

async def _method_async(self, request):
    # Async code here
    return response
```

**Message size configuration:**
- Set options on server creation (not environment variables)
- Max value limit: 2^28 (268MB) - larger values default to 4MB

### 3. Go Client

**Critical:** Always use DefaultConfig() for message size limits:
```go
// WRONG - creates Config with zero values
cfg := &grpcclient.Config{Target: target}

// CORRECT - starts with DefaultConfig()
cfg := grpcclient.DefaultConfig()
cfg.Target = target
```

**Dial options:**
```go
dialOpts := []grpc.DialOption{
    grpc.WithDefaultCallOptions(
        grpc.MaxCallRecvMsgSize(100 * 1024 * 1024),
        grpc.MaxCallSendMsgSize(100 * 1024 * 1024),
    ),
    grpc.WithTransportCredentials(insecure.NewCredentials()),
}
```

**Nil checks for response fields:**
```go
if resp.Metadata != nil {
    // Access nested fields
}
```

## Common Issues & Solutions

### Issue: "ResourceExhausted: trying to send message larger than max (X vs 0)"

**Cause:** Client's MaxSendMsgSize is 0 (not configured)

**Solution:** Use `DefaultConfig()` instead of creating empty Config struct

### Issue: Python async server fails on Windows

**Cause:** grpcio async server has Windows-specific bugs

**Solution:** Use sync server with `asyncio.run()` wrappers

### Issue: "trying to send message larger than max (X vs 4194304)"

**Cause:** Default limit is 4MB, message exceeds this

**Solution:** Increase both client and server limits (max 268MB for stability)

### Issue: Nil pointer dereference on response metadata

**Cause:** Server doesn't set optional fields

**Solution:** Always check `if resp.Field != nil` before accessing

## Project Structure

```
services/cognida-python/
├── proto/
│   └── service.proto          # Proto definition
├── grpc_service/
│   ├── servicer.py            # Server implementation
│   └── server.py              # Server entry point
└── proto/
    ├── service_pb2.py         # Generated protobuf
    └── service_pb2_grpc.py    # Generated gRPC

services/cognida-go/
├── api/proto/
│   ├── service.proto          # Copy of proto (with go_package)
│   ├── service.pb.go          # Generated protobuf
│   └── service_grpc.pb.go     # Generated gRPC
└── internal/infrastructure/grpc/
    └── service/
        ├── client.go          # Client implementation
        └── client_test.go     # Integration tests
```

## Code Generation

**Python:**
```bash
python -m grpc_tools.protoc \
    --python_out=. \
    --grpc_python_out=. \
    --proto_path=. proto/service.proto
```

**Go:**
```bash
protoc \
    --go_out=. \
    --go_opt=paths=source_relative \
    --go-grpc_out=. \
    --go-grpc_opt=paths=source_relative \
    proto/service.proto
```

## Testing

Always test with real server-client connection:
```bash
# Start Python server
python -m grpc_service.server

# Run Go tests
go test -v ./internal/infrastructure/grpc/service/
```
