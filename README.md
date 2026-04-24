# Agent VM

AI Agent 虚拟化管理平台 — 类似 nvm/Docker 的 Agent 环境管理工具

## 当前开发

```bash
go run ./cmd/avm --help
make build
make test
```

## Phase 1 目标体验

```bash
# 安装
brew install avm

# 初始化
avm init

# 创建 Agent Profile
avm agent create backend-coder --runtime codex

# 切换 Agent Profile
avm use backend-coder
```

## 文档

详见 [项目文档](../../ai-startup/projects/agent-vm/)

## 开发

```bash
go run ./cmd/avm --help
make build
make test
```
