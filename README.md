# MCP Bridge

把本地 `stdio` MCP 服务桥接成可直接给 Agent 使用的 `Streamable HTTP` MCP 服务，并提供一个内嵌到二进制里的 Web 管理台。

![管理台预览](./preview.png)

## 你可以用它做什么

- 用一个 JSON 配置文件管理多个本地 MCP 服务
- 通过单个 HTTP 端口暴露多个 MCP 路由
- 每个后端进程常驻复用，不会每个请求都重新拉起
- 支持管理台查看、编辑、测试和复制 Agent 配置
- 修改 MCP 配置后可热更新，无需重启整个服务

## 使用步骤

### 1. 下载并解压 release

下载对应平台的压缩包并解压后，你会得到：

- `mcp-bridge` 或 `mcp-bridge.exe`
- `config.json`
- `README.md`

其中 `config.json` 默认是空白配置：

```json
{
  "mcpServers": {}
}
```

### 2. 启动服务

macOS / Linux:

```bash
./mcp-bridge -config ./config.json -listen :8082
```

Windows:

```powershell
.\mcp-bridge.exe -config .\config.json -listen :8082
```

### 3. 打开管理台

启动后访问：

- 管理台：`http://127.0.0.1:8082/_admin/`
- 健康检查：`http://127.0.0.1:8082/healthz`

### 4. 通过 UI 添加 MCP

进入管理台后，直接使用界面完成配置：

- 点击“添加 MCP”
- 选择“表单添加”或 “JSON 添加”
- 填写命令、参数、环境变量、超时等信息
- 点击“测试”确认能成功初始化
- 点击“保存并热更新”立即生效

也就是说，正常使用时你不需要手动编辑 `config.json`，直接通过 UI 维护即可。

### 5. 配置到 Agent

添加完成后，可以在管理台里直接预览并复制 Agent 配置。

如果某个 MCP 的路由是 `/mcp/filesystem`，那么给 Agent 使用的配置类似：

```json
{
  "mcpServers": {
    "filesystem": {
      "transport": "streamable-http",
      "url": "http://127.0.0.1:8082/mcp/filesystem"
    }
  }
}
```

## 管理台里需要填什么

虽然通常不需要手改 `config.json`，但你在 UI 里仍然会看到这些字段：

常用字段说明：

- `enabled`：是否启用，`false` 时不会对外暴露
- `command`：启动命令，例如 `npx`、`uvx`、本地可执行文件路径
- `args`：命令参数数组
- `env`：环境变量对象，可省略
- `description`：说明文字，可省略
- `timeout`：超时时间，单位毫秒
- `path`：HTTP 路由，可省略；留空时会自动生成

自动生成路由示例：

- `filesystem` -> `/mcp/filesystem`
- `fetch` -> `/mcp/fetch`

## 手动编辑配置

如果你确实想直接修改 `config.json`，结构仍然是下面这样：

### `npx` 示例

```json
{
  "mcpServers": {
    "filesystem": {
      "enabled": true,
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"],
      "timeout": 60000
    }
  }
}
```

### `uvx` 示例

```json
{
  "mcpServers": {
    "fetch": {
      "enabled": true,
      "command": "uvx",
      "args": ["mcp-server-fetch", "--ignore-robots-txt"],
      "env": {
        "PYTHONIOENCODING": "utf-8"
      },
      "timeout": 60000
    }
  }
}
```

## 常用访问地址

- `/`：查看当前桥接路由
- `/healthz`：查看健康状态
- `/_admin/`：打开管理台
- `/mcp/<route>`：实际给 Agent 使用的 HTTP MCP 入口

## 注意事项

- 修改 MCP 配置后通常不需要重启服务
- 修改监听端口后需要重启服务
- 当所有 MCP 都被禁用时，服务仍然可以启动，但不会暴露可用路由

## 其他文档

- [示例配置](./config.example.json)
- [开发与构建指南](./DEVELOPMENT.md)
