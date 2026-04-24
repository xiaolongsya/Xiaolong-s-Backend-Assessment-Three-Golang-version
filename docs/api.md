# API 文档（OpenAI 风格 / MVP）

## 基本信息

- Base URL（本地默认）：`http://localhost:8091`
- 鉴权：所有接口都需要 Header：`Authorization: Bearer <token>`
  - 默认（未配置环境变量时）：`Authorization: Bearer test-token`
  - 可通过环境变量 `API_TOKENS` 配置允许的 token（逗号分隔）
- Content-Type：请求体为 JSON 的接口使用 `Content-Type: application/json`

可选（上游转发）：

- 当配置环境变量 `UPSTREAM_BASE_URL` 与 `UPSTREAM_API_KEY` 时：
  - `POST /v1/chat/completions` 会转发到上游 OpenAI 兼容接口（例如 MiniMax：`https://api.minimaxi.com/v1`）
  - 服务端会将上游返回的 `id` 统一改写为本服务生成的 `completion_id`，以确保 GET/DELETE/CANCEL 等生命周期接口一致

可选加分（API 中转：多 provider / 多 key）：

- 以数据库 `ai_models.owned_by` 作为 provider 标识（例如 `minimax/openai/aliyun/volcano`），服务端按 `owned_by` 自动选择对应上游配置
- 同一 provider 可配置多个 key（逗号分隔），服务端会按 `completion_id` 做确定性选择（便于多 key 分摊）

环境变量优先级（高 → 低）：

- `UPSTREAM_<PROVIDER>_BASE_URL` → `UPSTREAM_BASE_URL`
- `UPSTREAM_<PROVIDER>_API_KEYS` → `UPSTREAM_<PROVIDER>_API_KEY` → `UPSTREAM_API_KEY`

可选（上游不可用自动回退）：

- `UPSTREAM_FALLBACKS`：当 primary provider 不可用时，按配置顺序回退到其他 provider

格式：

```
primary=fallback1,fallback2;another=fallbackX
```

示例：

```powershell
$env:UPSTREAM_FALLBACKS='volcano=minimax;minimax=volcano'
```

## 通用错误返回

鉴权失败（401）：

```json
{
  "error": {
    "message": "Missing or invalid Authorization header",
    "type": "authentication_error"
  }
}
```

或：

```json
{
  "error": {
    "message": "Invalid API key",
    "type": "authentication_error"
  }
}
```

资源不存在（404）：

```json
{
  "error": {
    "message": "Not found",
    "type": "not_found"
  }
}
```

---

## 1) 健康检查

### `GET /healthz`

请求：

```bash
curl -H "Authorization: Bearer test-token" http://localhost:8091/healthz
```

响应：

```json
{ "status": "ok" }
```

---

## 2) Chat Completions

### `POST /v1/chat/completions`

支持字段：
- `model`（string）
- `messages`（array）
- `temperature`（number）
- `stream`（boolean）

模型白名单：

- `model` 必须存在于数据库表 `ai_models` 且 `enabled=1`
- 否则返回 `400 Bad Request`

```json
{
  "error": {
    "message": "Model not available",
    "type": "invalid_request_error"
  }
}
```

#### 2.1 非流式（`stream=false`）

请求：

```bash
curl -H "Authorization: Bearer test-token" \
  -H "Content-Type: application/json" \
  -X POST http://localhost:8091/v1/chat/completions \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "Hello"}],
    "temperature": 0.7,
    "stream": false
  }'
```

响应（示例）：

```json
{
  "id": "chatcmpl-...",
  "object": "chat.completion",
  "created": 1710000000,
  "model": "gpt-4o-mini",
  "choices": [
    {
      "index": 0,
      "message": {"role": "assistant", "content": "Hello! This is a mock response."},
      "finish_reason": "stop"
    }
  ]
}
```

#### 2.2 流式（`stream=true`，SSE）

- Response Header：`Content-Type: text/event-stream`
- 每个事件为一行 `data: <json>`，以空行分隔
- 结束以：`data: [DONE]`

请求：

```bash
curl -N -H "Authorization: Bearer test-token" \
  -H "Content-Type: application/json" \
  -X POST http://localhost:8091/v1/chat/completions \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "Stream test"}],
    "temperature": 0.7,
    "stream": true
  }'
```

流式 chunk（示例）：

```text
data: {"id":"chatcmpl-...","object":"chat.completion.chunk","created":1710000000,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"role":"assistant"}}]}

data: {"id":"chatcmpl-...","object":"chat.completion.chunk","created":1710000001,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"content":"chunk-01 "}}]}

...

data: [DONE]
```

---

## 3) 生成结果管理

### `GET /v1/chat/completions/:id`

说明：按 `completion_id` 查询结果。服务端返回落库的 `response`（JSON）。

请求：

```bash
curl -H "Authorization: Bearer test-token" \
  http://localhost:8091/v1/chat/completions/chatcmpl-xxxxx
```

响应：为一次 `chat.completion` 的 JSON。

### `DELETE /v1/chat/completions/:id`

请求：

```bash
curl -H "Authorization: Bearer test-token" \
  -X DELETE http://localhost:8091/v1/chat/completions/chatcmpl-xxxxx
```

响应（示例）：

```json
{
  "id": "chatcmpl-xxxxx",
  "object": "chat.completion.deleted",
  "deleted": true
}
```

### `POST /v1/chat/completions/:id/cancel`

说明：用于取消一个正在进行的 `stream=true` 任务。

请求：

```bash
curl -H "Authorization: Bearer test-token" \
  -X POST http://localhost:8091/v1/chat/completions/chatcmpl-xxxxx/cancel
```

响应（示例）：

```json
{
  "id": "chatcmpl-xxxxx",
  "object": "chat.completion.cancelled",
  "cancelled": true
}
```

---

## 4) Models

### `GET /v1/models`（兼容 `POST /v1/models`）

说明：模型列表来自数据库表 `ai_models`，仅返回 `enabled=1` 的模型。

请求：

```bash
curl -H "Authorization: Bearer test-token" http://localhost:8091/v1/models
```

响应（示例）：

```json
{
  "object": "list",
  "data": [
    {
      "id": "gpt-4o-mini",
      "object": "model",
      "created": 1710000000,
      "owned_by": "organization"
    }
  ]
}
```

---

## 5) Files（可选加分）

说明：用于管理上传文件资源（落盘 + 元信息入库）。

- 上传大小限制：20MB
- 超限返回：`413 Payload Too Large`

### `POST /v1/files`

请求：`multipart/form-data`

- `file`：文件（必填）
- `purpose`：用途（可选字符串，例如 `assistants`）

Postman：Body 选择 `form-data`，把 `file` 的类型切换为 File。

响应：

```json
{
  "id": "file-...",
  "object": "file",
  "bytes": 123,
  "created_at": 1710000000,
  "filename": "demo.txt",
  "purpose": "assistants"
}
```

### `GET /v1/files`

响应：

```json
{
  "object": "list",
  "data": [
    {
      "id": "file-...",
      "object": "file",
      "bytes": 123,
      "created_at": 1710000000,
      "filename": "demo.txt",
      "purpose": "assistants"
    }
  ]
}
```

### `GET /v1/files/{file_id}`

返回单个文件元信息，结构同 `POST /v1/files`。

### `DELETE /v1/files/{file_id}`

响应：

```json
{
  "id": "file-...",
  "object": "file",
  "deleted": true
}
```
