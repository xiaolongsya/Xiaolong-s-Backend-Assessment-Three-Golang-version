# OpenAI 风格后端（Go + Gin + MySQL）

本项目实现（部分）OpenAI API 风格接口，支持：
- Bearer Token 鉴权
- Chat Completions（流式 SSE / 非流式）
- 生成结果管理（GET / DELETE / CANCEL）
- Models 列表
- MySQL 持久化（GORM AutoMigrate）

## 快速开始（本地）

### 1) 配置环境变量

必须：
- `MYSQL_DSN`：MySQL DSN（项目启动时强校验）

鉴权（推荐配置）：
- `API_TOKENS`：允许访问的 Bearer Token 列表（逗号分隔）。未配置时默认允许 `test-token`，以便本地/考核脚本直接运行。

示例（PowerShell）：

```powershell
$env:MYSQL_DSN='user:pass@tcp(host:3306)/kaohe3-go?charset=utf8mb4&parseTime=True&loc=Local'
```

可选：
- `TZ=Asia/Shanghai`：推荐用于容器部署，避免时间少 8 小时（UTC）的问题

### 2) 启动服务

```bash
go run ./cmd
```

默认监听：`http://localhost:8091`

### 3) 健康检查

```powershell
curl.exe -H "Authorization: Bearer test-token" http://localhost:8091/healthz
```

## SDK 回归测试

```powershell
python tests/sdk_test.py
```

预期输出包含：`models.list ok`、`chat non-stream ok`、`chat stream ok`、`ALL OK`。

## 文档

- 接口文档：`docs/api.md`
- Push 说明：`docs/push_notes.md`

## 1Panel 部署提示（简要）

- 如果用 1Panel 的 Go 运行环境（容器）并选择 `go run`/`go build`：服务器需要能访问 Go Module 代理，否则会出现 `proxy.golang.org ... i/o timeout`。
  - 可通过设置运行环境变量 `GOPROXY=https://goproxy.cn,direct` 解决
  - 或者更稳：本地编译 Linux 二进制后上传，启动命令用 `./your-binary`
- MySQL 若为 1Panel 容器：后端容器需加入同一网络（例如 `1panel-network`），并在 `MYSQL_DSN` 里用 `mysql:3306`（服务名）连接。
