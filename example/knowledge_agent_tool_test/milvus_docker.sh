#!/bin/bash

# Milvus Docker Compose 管理脚本

COMPOSE_FILE="docker-compose-milvus.yml"

case "$1" in
  up)
    echo "启动 Milvus 服务..."
    docker-compose -f $COMPOSE_FILE up -d
    echo "Milvus 服务已启动"
    ;;
  down)
    echo "停止并移除 Milvus 服务..."
    docker-compose -f $COMPOSE_FILE down
    echo "Milvus 服务已停止"
    ;;
  restart)
    echo "重启 Milvus 服务..."
    docker-compose -f $COMPOSE_FILE down
    docker-compose -f $COMPOSE_FILE up -d
    echo "Milvus 服务已重启"
    ;;
  logs)
    echo "查看 Milvus 日志..."
    docker-compose -f $COMPOSE_FILE logs -f milvus
    ;;
  status)
    echo "查看 Milvus 服务状态..."
    docker-compose -f $COMPOSE_FILE ps
    ;;
  *)
    echo "使用方法: $0 {up|down|restart|logs|status}"
    echo "  up      - 启动 Milvus 服务"
    echo "  down    - 停止并移除 Milvus 服务"
    echo "  restart - 重启 Milvus 服务"
    echo "  logs    - 查看 Milvus 日志"
    echo "  status  - 查看服务状态"
    exit 1
    ;;
esac