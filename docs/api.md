# API 文档（OpenAI 风格 / MVP）

## 基本信息

- Base URL（本地默认）：`http://localhost:8091`
- 鉴权：所有接口都需要 Header：`Authorization: Bearer <token>`
  - 默认（未配置环境变量时）：`Authorization: Bearer test-token`
  - 可通过环境变量 `API_TOKENS` 配置允许的 token（逗号分隔）
- Content-Type：请求体为 JSON 的接口使用 `Content-Type: application/json`

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
