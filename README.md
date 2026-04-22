# Agent VM

AI Agent 虚拟化管理平台 — 类似 nvm/Docker 的 Agent 环境管理工具

## 快速开始

```bash
# 安装
brew install avm

# 初始化
avm init

# 创建环境
avm env create coding --skills git,test,review --mcps github,jira,db

# 切换环境
avm use coding
```

## 文档

详见 [项目文档](../../ai-startup/projects/agent-vm/)

## 开发

```bash
make build
make test
```
