# OpenAI 风格后端（Go + Gin + MySQL）

本项目实现（部分）OpenAI API 风格接口，支持：
- Bearer Token 鉴权
- Chat Completions（流式 SSE / 非流式）
- 生成结果管理（GET / DELETE / CANCEL）
- Models 列表
- MySQL 持久化（GORM AutoMigrate）

## 架构简述

- `cmd/main.go`：路由注册与服务启动
- `internal/middleware/auth.go`：Bearer Token 鉴权中间件（统一拦截所有接口）
- `internal/handler/`：业务 Handler（chat/models/任务管理）
- `internal/model/`：GORM 模型与数据库初始化（MySQL）

## 鉴权与生成流程说明

- 请求进入后先经过鉴权中间件：校验 `Authorization: Bearer <token>`，失败返回 `401`
- `POST /v1/chat/completions`：解析请求体 → 校验 `model` 是否在 `ai_models(enabled=1)` 白名单 → 生成 `completion_id` → 记录请求/响应到 MySQL
- `stream=true`：使用 SSE（`text/event-stream`）逐块输出，支持 `POST /v1/chat/completions/{id}/cancel` 取消
- `GET/DELETE`：按 `completion_id` 查询/删除已持久化的结果

## 快速开始（本地）

### 1) 配置环境变量

必须：
- `MYSQL_DSN`：MySQL DSN（项目启动时强校验）

鉴权（推荐配置）：
- `API_TOKENS`：允许访问的 Bearer Token 列表（逗号分隔）。未配置时默认允许 `test-token`，以便本地/考核脚本直接运行。

上游转发（可选）：
- `UPSTREAM_BASE_URL`：上游 OpenAI 兼容 Base URL（示例：`https://api.minimaxi.com/v1`）
- `UPSTREAM_API_KEY`：上游 API Key（将作为 `Authorization: Bearer <key>` 转发）
  - 未配置时：服务端使用 mock 逻辑返回（仍支持流式/非流式，便于本地与考核脚本验证）

上游转发（进阶：多 key / 多提供商，推荐）：

- 以数据库 `ai_models.owned_by` 作为“提供商标识”（例如 `minimax` / `openai` / `aliyun`）
- 后端会按 `owned_by` 自动选择上游配置，支持同一提供商配置多个 key（逗号分隔）

环境变量优先级（高 → 低）：

- `UPSTREAM_<PROVIDER>_BASE_URL` → `UPSTREAM_BASE_URL`
- `UPSTREAM_<PROVIDER>_API_KEYS`（多 key）→ `UPSTREAM_<PROVIDER>_API_KEY`（单 key）→ `UPSTREAM_API_KEY`

示例（PowerShell）：

```powershell
$env:UPSTREAM_MINIMAX_BASE_URL='https://api.minimaxi.com/v1'
$env:UPSTREAM_MINIMAX_API_KEYS='key-1,key-2,key-3'

$env:UPSTREAM_OPENAI_BASE_URL='https://api.openai.com/v1'
$env:UPSTREAM_OPENAI_API_KEY='sk-xxxx'
```

对应插入模型示例：

```sql
INSERT INTO ai_models (model_id, owned_by, enabled, created)
VALUES ('MiniMax-M2.7', 'minimax', 1, UNIX_TIMESTAMP());

INSERT INTO ai_models (model_id, owned_by, enabled, created)
VALUES ('gpt-4o-mini', 'openai', 1, UNIX_TIMESTAMP());
```
示例（PowerShell）：

```powershell
$env:MYSQL_DSN='user:pass@tcp(host:3306)/kaohe3-go?charset=utf8mb4&parseTime=True&loc=Local'
```

可选：
- `TZ=Asia/Shanghai`：推荐用于容器部署，避免时间少 8 小时（UTC）的问题

### 1.5) 模型白名单（ai_models）

- `/v1/models` 返回数据库表 `ai_models` 中 `enabled=1` 的模型
- `POST /v1/chat/completions` 的 `model` 必须存在于 `ai_models` 且 `enabled=1`，否则返回 `400 Bad Request`（`Model not available`）

示例（MySQL）：

```sql
INSERT INTO ai_models (model_id, owned_by, enabled, created)
VALUES ('MiniMax-M2.7', 'minimax', 1, UNIX_TIMESTAMP());
```

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

## AIGC 使用说明

- 使用工具：GitHub Copilot（用于辅助生成/改写部分代码与文档草稿）
- 使用范围：OpenAPI（Swagger/Apifox）导出文件、接口文档措辞、以及部分样板代码整理
- 说明：所有关键逻辑（鉴权、白名单校验、流式输出、落库与生命周期接口）均由人工审阅并在本地/线上联调验证

## 文档

- 接口文档：`docs/api.md`
- Push 说明：`docs/push_notes.md`

## 1Panel 部署提示（简要）

- 如果用 1Panel 的 Go 运行环境（容器）并选择 `go run`/`go build`：服务器需要能访问 Go Module 代理，否则会出现 `proxy.golang.org ... i/o timeout`。
  - 可通过设置运行环境变量 `GOPROXY=https://goproxy.cn,direct` 解决
  - 或者更稳：本地编译 Linux 二进制后上传，启动命令用 `./your-binary`
- MySQL 若为 1Panel 容器：后端容器需加入同一网络（例如 `1panel-network`），并在 `MYSQL_DSN` 里用 `mysql:3306`（服务名）连接。
