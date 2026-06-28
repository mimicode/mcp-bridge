# 开发指南

本文档面向项目维护者和二次开发者，包含本地开发、前端构建、打包发布与热更新机制说明。

## 项目结构

```text
.
├── cmd/mcp-bridge          # 程序入口
├── internal/bridge         # MCP bridge 核心逻辑
├── internal/config         # 配置加载与校验
├── internal/ui             # 内嵌前端产物
├── web                     # 管理台前端项目（Vue 3 + Vite + Naive UI）
├── scripts/dev.sh          # 本地开发脚本
├── scripts/package-all.sh  # 一键打包脚本
├── config.example.json     # 示例配置
└── README.md               # 面向使用者的说明
```

## 本地开发

### 后端开发

直接运行：

```bash
go run ./cmd/mcp-bridge -config ./config.json -listen :8082
```

可用参数：

- `-config`：MCP 配置文件路径
- `-listen`：HTTP 监听地址
- `-base-path`：自动生成路由时使用的基础路径
- `-version`：输出构建信息并退出

### 一键开发脚本

推荐使用：

```bash
./scripts/dev.sh
```

脚本会自动：

- 安装前端依赖
- 构建管理台前端
- 启动 Go 服务

支持环境变量覆盖：

```bash
CONFIG_PATH=./config.example.json LISTEN_ADDR=:8080 BASE_PATH=/mcp ./scripts/dev.sh
```

支持命令行参数：

```bash
./scripts/dev.sh --config ./config.example.json --listen :8080 --base-path /mcp
```

常用参数：

```bash
./scripts/dev.sh --skip-build
./scripts/dev.sh --skip-install
./scripts/dev.sh --help
```

## 前端开发

管理台位于 `web/` 目录，技术栈为 `Vue 3 + Vite + Naive UI`。

手动启动前端构建流程：

```bash
cd web
npm install
npm run build
```

说明：

- 构建产物输出到 `internal/ui/dist`
- Go 程序通过 `embed.FS` 内嵌这些静态资源
- 修改前端后，如果是通过 Go 二进制访问管理台，需要重新构建并重启进程

## 打包发布

一键打包脚本：

```bash
./scripts/package-all.sh
```

脚本会：

- 在 `web/` 下执行 `npm ci && npm run build`
- 可选执行 `go test ./...`
- 交叉编译 macOS / Linux / Windows 二进制
- 将 `README.md` 和 `config.example.json` 一并打包
- 在打包前清空并重建 `release/` 目录

默认目标平台：

- `darwin/amd64`
- `darwin/arm64`
- `linux/amd64`
- `linux/arm64`
- `windows/amd64`
- `windows/arm64`

可选覆盖参数：

```bash
VERSION=v0.1.0 TARGETS="darwin/arm64 linux/amd64 windows/amd64" ./scripts/package-all.sh
```

默认行为：

- `VERSION=dev`
- 若未覆盖 `VERSION`，重复构建时输出文件名保持稳定

## 构建信息

打包时会向二进制注入以下信息：

- `Version`
- `Commit`
- `BuildTime`

运行时查看：

```bash
./mcp-bridge -version
```

## 测试

运行全部 Go 测试：

```bash
go test ./...
```

检查是否可以完整编译：

```bash
go build ./...
```

## 热更新机制

- 在管理台保存配置时，会重写配置文件并只热更新 MCP 路由
- 未变化的路由会复用原有后端进程
- 已删除或已修改的路由会被重建或关闭
- 监听端口来自进程启动参数，变更后仍需重启进程

## 说明

- 默认优先面向运行时版本的 Vue，入口需使用 render 函数而不是 template 字符串
- 管理台主题支持亮色、暗黑、跟随系统，并持久化到浏览器本地存储
