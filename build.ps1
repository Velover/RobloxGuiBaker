#!/bin/bash
# Build for current platform
go build -o bin/gui-baker.exe apps/gui-baker/gui-baker.go
go build -o bin/plugin-connector.exe apps/plugin-connector/plugin-connector.go

echo "✓ Build complete!"
