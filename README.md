# 环境变量配置说明

本项目所有运行参数均通过 .env 文件集中配置，考核包已内置示例，考核者无需手动设置环境变量。

**主要环境变量说明：**

- `MYSQL_DSN`：MySQL 连接串，已配置为云端数据库，无需本地安装 MySQL。
  - 示例：`MYSQL_DSN=xiaolong:2ethZZBiMQEw5Ppj@tcp(xiaolongya.cn:3306)/kaohe3-go?charset=utf8mb4&parseTime=True&loc=Local`
- `API_TOKENS`：允许访问的 Bearer Token，多个用逗号分隔。默认已配置。
- `UPSTREAM_BASE_URL` / `UPSTREAM_API_KEY`：上游 OpenAI 兼容服务地址与密钥（如需转发）。
- `UPSTREAM_<PROVIDER>_BASE_URL` / `UPSTREAM_<PROVIDER>_API_KEYS`：多 provider 支持，按 ai_models.owned_by 自动选择。
- `UPSTREAM_FALLBACKS`：上游不可用时的回退链配置。
- `FILE_STORAGE_DIR`：文件上传存储目录，未配置时默认写入 ./data/files。
- `GOPROXY`：Go 依赖代理，国内建议保持默认。

**使用说明：**

1. 直接运行 backend.exe 即可，程序会自动加载同目录下的 .env 文件。
2. 所有环境变量均可在 .env 文件中修改，无需手动 export/set。
3. 如需自定义数据库或 token，仅需编辑 .env 文件对应项。

**安全提示：**

如用于正式环境，请更换为专用数据库账号和 token，考核结束后可删除云端数据库或更改密码。

# OpenAI 风格后端（Go + Gin + MySQL）

本项目实现（部分）OpenAI API 风格接口，支持：
- Bearer Token 鉴权
- Chat Completions（流式 SSE / 非流式）
- 生成结果管理（GET / DELETE / CANCEL）
- Models 列表
- MySQL 持久化（GORM AutoMigrate）

可选加分：
- Files API（文件上传/列表/元信息/删除；落盘 + 元信息入库；上传限制 20MB）
- API 中转（多 provider / 多 key / 回退；按 `ai_models.owned_by` 选择上游）
- 支持将模型生成请求转发给真实的模型服务
- 至少支持两种不同的模型服务中转（比如 MINIMAX和火山引擎的deepseekv3.2）
- 通过环境变量存储这些模型服务的鉴权密钥
- 实现 WebUI 实现AI对话和文件管理


## 架构简述

- `cmd/main.go`：路由注册与服务启动
- `internal/middleware/auth.go`：Bearer Token 鉴权中间件（统一拦截所有接口）
- `internal/handler/`：HTTP 适配层（Gin handler：参数绑定 / 返回 JSON 或 SSE）
- `internal/service/`：业务编排层（模型白名单校验、上游选择/回退、落库状态机）
- `internal/repo/`：数据访问层（基于 GORM 的 DB 读写封装）
- `internal/upstream/`：上游客户端层（OpenAI 兼容 HTTP 调用/流式连接）
- `internal/task/`：任务注册层（流式任务 cancel/finish 管理）
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

上游不可用自动回退（可选）：

- `UPSTREAM_FALLBACKS`：当 primary provider 不可用时，按配置顺序回退到其他 provider

格式：

```
primary=fallback1,fallback2;another=fallbackX
```

示例（火山引擎与 MiniMax 双向回退）：

```powershell
$env:UPSTREAM_FALLBACKS='volcano=minimax;minimax=volcano'
```

触发回退的情况（最小实现）：网络错误、HTTP 5xx、429、401、403。

### API 中转（可选加分：已实现部分）

对照 `openai.md` 的“API 中转”方向，本项目已实现：

- 将 `POST /v1/chat/completions` 转发到真实上游（OpenAI 兼容）
- 多 provider 配置（按 `ai_models.owned_by` 选择 `UPSTREAM_<PROVIDER>_*`）
- 同一 provider 多 key（`UPSTREAM_<PROVIDER>_API_KEYS`）与自动回退（`UPSTREAM_FALLBACKS`）

未实现：严格限流/限额管理、WebUI 配置管理。

详细说明：见下方「API 中转（可选加分）」章节。

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

Files（可选加分）：

- `FILE_STORAGE_DIR`：上传文件的落盘目录
  - 未配置时：默认写入 `./data/files`
  - Linux 部署示例：`/long_app/backend3-go-file`

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

可通过环境变量覆盖：

- `OPENAI_BASE_URL`：默认 `http://xiaolongya.cn:8091/v1`，本地运行请改成 `http://localhost:8091/v1`
- `OPENAI_API_KEY`：默认 `test-token`

说明：SDK 脚本当前默认面向云端验收地址；如果在本地联调，建议显式设置 `OPENAI_BASE_URL=http://localhost:8091/v1` 再执行。

## API 中转（可选加分）

用于解决不同模型提供商 API/SDK 不统一的问题：后端对外保持 OpenAI 风格接口，对内按 provider 配置转发到真实上游（上游需提供 OpenAI 兼容的 `/chat/completions`）。

已实现能力：

- 多 provider：以数据库 `ai_models.owned_by` 作为 provider 标识，按 `owned_by` 选择 `UPSTREAM_<PROVIDER>_*` 配置
- 多 key：支持 `UPSTREAM_<PROVIDER>_API_KEYS=key1,key2,...`，服务端按 `completion_id` 做“确定性选 key”以便分摊
- 自动回退：支持 `UPSTREAM_FALLBACKS` 指定回退链（例如 `volcano=minimax;minimax=volcano`）
  - 触发条件（最小实现）：网络错误、HTTP 5xx、429、401、403
- 生命周期一致性：上游成功响应会被改写 `id` 为本服务生成的 `completion_id`，确保后续 GET/DELETE/CANCEL 能正确工作

配置方式与示例：见上方“配置环境变量 → 上游转发/回退”段落。

示例（MINIMAX + 火山引擎 deepseekv3.2）：

```powershell
$env:UPSTREAM_MINIMAX_BASE_URL='https://api.minimaxi.com/v1'
$env:UPSTREAM_MINIMAX_API_KEYS='key-1,key-2'

$env:UPSTREAM_VOLCANO_BASE_URL='https://<your-volcano-openai-compatible-base>/v1'
$env:UPSTREAM_VOLCANO_API_KEY='vk-xxxx'

$env:UPSTREAM_FALLBACKS='minimax=volcano;volcano=minimax'
```

未实现（不在交付承诺范围）：严格限流/限额管理。

## Admin API（可选加分：已实现部分）

用于初始化/运维 `ai_models`：

- `GET /v1/admin/models`：列出所有模型（包含 `enabled=0/1`）
- `PATCH /v1/admin/models/{model_id}`：更新某个模型是否启用（请求体：`{"enabled": true|false}`）

## Files API（可选加分）

用于管理上传文件资源（文件内容落盘 + 元信息入库），接口风格参考 OpenAI Files API。

WebUI 对应：项目提供 WebUI 的“文件管理”面板（上传/列表/详情/删除），即调用本节的 `/v1/files` 系列接口。

### 1) 上传文件

- `POST /v1/files`
- 请求类型：`multipart/form-data`
- 表单字段：
  - `file`：文件（File）
  - `purpose`：用途（Text，字符串，允许为空）

限制：

- 上传大小限制：20MB
- 超限返回：`413 Payload Too Large`

返回：文件对象（`object=file`），包含 `id / bytes / created_at / filename / purpose` 等字段。

### 2) 文件列表

- `GET /v1/files`

返回：`{ object: "list", data: [...] }`

### 3) 获取文件元信息

- `GET /v1/files/{file_id}`

### 4) 删除文件

- `DELETE /v1/files/{file_id}`

返回：`{ id, object: "file", deleted: true }`

### Postman 测试要点

- 所有请求都需要 Header：`Authorization: Bearer <token>`
- 上传请求 Body 选择 `form-data`，并把 `file` 的类型切换为 File

## AIGC 使用说明

- 使用工具：GitHub Copilot（用于辅助生成/改写部分代码与文档草稿）
- 使用范围：OpenAPI（Swagger/Apifox）导出文件、接口文档措辞、以及部分样板代码整理
- 说明：所有关键逻辑（鉴权、白名单校验、流式输出、落库与生命周期接口）均由人工审阅并在本地/线上联调验证

## 文档

- 接口文档：`docs/api.md`
- Push 说明：`docs/push_notes.md`
- OpenAPI：`docs/openapi.yaml`

## WebUI（可选加分）

提供一个 Vue3/Vite 的 WebUI，用于：

- AI 对话（调用 `/v1/chat/completions`，支持流式/非流式）
- 文件管理（调用 `/v1/files`：上传/列表/详情/删除）

说明：WebUI 为独立前端工程（例如同级目录 `backend3-go-fronted`），通过环境变量配置后端地址与 token。

- `VITE_API_BASE_URL`：后端 base url（示例：`http://localhost:8091`）
- `VITE_API_TOKEN`：Bearer token（示例：`test-token`）

## 1Panel 部署提示（简要）

- 如果用 1Panel 的 Go 运行环境（容器）并选择 `go run`/`go build`：服务器需要能访问 Go Module 代理，否则会出现 `proxy.golang.org ... i/o timeout`。
  - 可通过设置运行环境变量 `GOPROXY=https://goproxy.cn,direct` 解决
  - 或者更稳：本地编译 Linux 二进制后上传，启动命令用 `./your-binary`
- MySQL 若为 1Panel 容器：后端容器需加入同一网络（例如 `1panel-network`），并在 `MYSQL_DSN` 里用 `mysql:3306`（服务名）连接。
