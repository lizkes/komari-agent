#!/bin/bash

# 生成 Go 代码
protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    monitor.proto

echo "protobuf代码生成完成" 