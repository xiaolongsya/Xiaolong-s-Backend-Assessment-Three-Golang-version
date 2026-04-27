# Push 说明（第三轮后端考核 / OpenAI 风格后端）

## 完成情况
- 实现 OpenAI 风格 API（Gin）：
  - `GET /healthz`
  - `POST /v1/chat/completions`（支持 `stream=true/false`）
  - `GET /v1/chat/completions/:id`
  - `DELETE /v1/chat/completions/:id`
  - `POST /v1/chat/completions/:id/cancel`
  - `GET /v1/models`（兼容 `POST /v1/models`）
- 全接口 Bearer Token 鉴权（中间件统一处理）；默认测试 token：`test-token`（也支持通过环境变量 `API_TOKENS` 配置）
- 生成请求全生命周期落库（GORM + MySQL）：
  - 自动建表 `completions`
  - 记录 request/response（`longtext`），并维护状态 `status`（含索引）
  - cancel 会写回 `status=cancelled` 且 `cancelled_at` 非空

## 扩展能力（实现）

- 上游中转（OpenAI 兼容）：可通过 `UPSTREAM_BASE_URL` + `UPSTREAM_API_KEY` 将 `/v1/chat/completions` 转发到真实模型服务（如 MiniMax）
- 上游不可用自动回退：通过 `UPSTREAM_FALLBACKS` 配置 provider 回退顺序；当网络错误、5xx、429、401、403 等情况触发回退
- 模型白名单（ai_models）：`/v1/models` 返回 `enabled=1` 的模型；chat 请求的 `model` 必须在白名单内，否则返回 `400 Model not available`

## API 中转（可选加分：已实现部分）

对照 `openai.md` 的“API 中转”方向，本项目已实现其中与工程落地最相关的一部分：

- 支持将 chat 请求转发到真实上游（OpenAI 兼容接口）
  - 实现位置：`internal/service/chat_service.go`（`ProxyNonStream`、`OpenUpstreamStream`）
- 至少支持两种不同上游“提供商”的中转（通过 provider 标识抽象实现，不硬编码厂商）
  - provider 标识来源：数据库 `ai_models.owned_by`
  - owned_by → provider 后缀规范化：`internal/service/upstream_config.go`（`EnvKeySuffixFromOwnedBy`）
  - 说明：可用 `minimax/openai/aliyun/volcano/...` 等作为 `owned_by`；只要配置对应 `UPSTREAM_<PROVIDER>_*` 即可接入
- 通过环境变量存储鉴权密钥，并支持同一 provider 多 key
  - `UPSTREAM_<PROVIDER>_API_KEYS`（逗号分隔）或 `UPSTREAM_<PROVIDER>_API_KEY` / `UPSTREAM_API_KEY`
  - key 选择策略：按 `completion_id` 做确定性挑选（用于分摊到多 key）：`internal/service/upstream_config.go`（`pickKeyDeterministic`）
- 上游不可用自动回退到其他 provider
  - 回退配置：`UPSTREAM_FALLBACKS`：`internal/service/upstream_config.go`（`ParseUpstreamFallbacksFromEnv`）
  - 回退条件：网络错误、HTTP 5xx、429、401、403：`internal/service/upstream_config.go`（`ShouldFallbackForStatus`）
- 生命周期一致性：上游返回的成功响应会把 `id` 改写为本服务生成的 `completion_id`
  - 实现位置：`internal/service/chat_service.go`（`ProxyNonStream` 的 id rewrite；流式在 `internal/handler/chat.go` 做 id rewrite）

未实现（未写入交付承诺）：

- 严格的限额管理/限流（例如按 token 或 IP 的 429 限流）
- WebUI（用于查看运行状态/配置管理）

## Files API（可选加分，已实现）

- `POST /v1/files` 上传文件（落盘 + 元信息入库）
  - 上传大小限制：20MB；超限返回 `413`
- `GET /v1/files` 文件列表
- `GET /v1/files/:file_id` 文件元信息
- `DELETE /v1/files/:file_id` 删除文件（硬删 DB + 删除落盘文件）

## Admin API（可选加分：已实现部分）

- `GET /v1/admin/models`：列出所有模型（包含 enabled=0/1）
- `PATCH /v1/admin/models/:model_id`：启用/禁用某个模型（请求体：`{"enabled": true|false}`）

## 分层与可维护性

- handler 保持薄：参数绑定 + 调用 service + 输出
- completion 的查询/删除已下沉到 repo/service（handler 不再直接访问 GORM DB）
- chat/models/files 的 DTO 统一收敛到 `internal/dto`

## 数据库切换（SQLite → MySQL）
- 当前仅支持 MySQL（不再保留 SQLite 依赖）
- 通过环境变量 `MYSQL_DSN` 配置数据库连接；启动时强校验，缺失即退出

## 本地运行
1. 配置环境变量（示例）
   - PowerShell：
     - `$env:MYSQL_DSN='user:pass@tcp(host:3306)/kaohe3-go?charset=utf8mb4&parseTime=True&loc=Local'`
2. 启动服务
   - `go run ./cmd`
3. 健康检查
  - `curl.exe -H "Authorization: Bearer test-token" http://localhost:8091/healthz`

## 验证步骤（已验证）
- 非流式生成：`POST /v1/chat/completions` 返回 `chatcmpl-...` 且写入 MySQL
- 流式生成：`stream=true` 可被官方 OpenAI Python SDK 正确解析
- 取消生成：调用 `POST /v1/chat/completions/{id}/cancel` 后，MySQL 中该记录 `status=cancelled` 且 `cancelled_at` 非空

## SDK 回归测试（已通过）
- 脚本：`tests/sdk_test.py`
- 运行：`python tests/sdk_test.py`
- 可选环境变量：`OPENAI_BASE_URL`（默认 `http://localhost:8091/v1`）、`OPENAI_API_KEY`（默认 `test-token`）
- 预期输出包含：`models.list ok`、`chat non-stream ok`、`chat stream ok`、`ALL OK`

## openai.md 对照表（静态验收）

说明：以下为对照 `openai.md` 的“静态验收证据点”（不依赖联调输出），便于评审快速定位实现。

### 1) 认证与鉴权（MVP）

- 全局鉴权中间件：`cmd/main.go`（`r.Use(middleware.AuthMiddleware())`）
- Bearer Token 校验与 401 错误体：`internal/middleware/auth.go`（`AuthMiddleware`）
- 默认 token 与环境变量：`internal/middleware/auth.go`（`API_TOKENS`，未配置默认 `test-token`）

### 2) Chat Completions（MVP）

- 路由：`cmd/main.go`（`POST /v1/chat/completions`）
- Handler：`internal/handler/chat.go`（`ChatCompletions`）
- 支持参数解析：`internal/dto/chat.go`（`model/messages/stream/temperature`）
- stream=true：SSE 输出 + `data: [DONE]`：`internal/handler/chat.go`（`ChatCompletions`）
- 非流式：JSON 输出：`internal/handler/chat.go`（`ChatCompletions`）
- 请求体非法时的 OpenAI 风格错误体：`internal/handler/chat.go`（`Invalid request body`，`invalid_request_error`）

### 3) 模型白名单（MVP）

- `/v1/models` 来源：`ai_models(enabled=1)`：`internal/handler/models.go`、`internal/service/model_service.go`、`internal/repo/ai_model_repo.go`
- chat 的 `model` 白名单校验：`internal/handler/chat.go`（调用 `chatSvc.GetEnabledModelOrErr`）

### 4) 生成结果管理（MVP）

- 路由：`cmd/main.go`
  - `GET /v1/chat/completions/:id`
  - `DELETE /v1/chat/completions/:id`
  - `POST /v1/chat/completions/:id/cancel`
- Handler：`internal/handler/chat.go`（`GetCompletion/DeleteCompletion/CancelCompletion`）
- 取消机制：`internal/task`（`Register/Cancel/Finish`）

### 5) DB 持久化（MVP）

- 初始化与迁移：`internal/model`（MySQL 初始化 + AutoMigrate）
- 创建 running：`internal/repo/completion_repo.go`（`CreateRunning`）
- 更新 completed/failed/cancelled：`internal/service/chat_service.go`（`MarkCompleted/MarkFailed/MarkCancelled`）→ `internal/repo/completion_repo.go`（`UpdateFields`）

### 6) Files API（可选加分）

- 路由：`cmd/main.go`（`POST/GET/GET by id/DELETE /v1/files`）
- Handler：`internal/handler/file.go`（`CreateFile/ListFiles/GetFile/DeleteFile`）
- 上传大小限制：20MB（超限 413）：`internal/handler/file.go`（`maxFileBytes` + `http.MaxBytesReader`）
- 落盘目录：`internal/service/file_service.go`（`FILE_STORAGE_DIR`，默认 `./data/files`）

### 7) 官方 SDK 可用性验证（MVP）

- 官方 OpenAI Python SDK：`tests/sdk_test.py`（`from openai import OpenAI`）
- 可配置 base_url / api_key：`tests/sdk_test.py`（`OPENAI_BASE_URL/OPENAI_API_KEY`）
- 覆盖 models + chat 非流式 + chat 流式：`tests/sdk_test.py`

## 提交摘要

- 已覆盖 openai.md 的 MVP 要求：鉴权、Chat Completions、模型白名单、结果持久化与生命周期管理、SDK 可用性验证
- 已实现加分项：Files API、API 中转（多 provider / 多 key / 回退）、Admin API、WebUI 文件管理与对话入口
- 构建状态：后端 `go build ./...` 通过；前端 `npm run build` 通过

## 已知限制

- `POST /v1/chat/completions/:id/cancel` 的任务取消目前采用进程内注册表，单实例可用，多实例需要进一步做分布式任务协调
- Files API 当前落盘到本地磁盘目录，适合考核与单机部署；生产环境建议接对象存储并补充总量配额
- `tests/sdk_test.py` 默认指向云端验收地址，本地复现时建议显式设置 `OPENAI_BASE_URL=http://localhost:8091/v1`
