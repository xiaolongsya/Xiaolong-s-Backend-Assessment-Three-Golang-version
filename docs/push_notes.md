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
- 模型白名单（ai_models）：`/v1/models` 返回 `enabled=1` 的模型；chat 请求的 `model` 必须在白名单内，否则返回 `400 Model not available`

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
- 预期输出包含：`models.list ok`、`chat non-stream ok`、`chat stream ok`、`ALL OK`
