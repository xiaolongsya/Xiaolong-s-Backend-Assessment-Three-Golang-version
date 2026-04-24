# 第三轮后端考核说明(智能应用方向)

## 考核背景

近年来，基于大语言模型（LLM）的对话系统被广泛应用于虚拟助手、角色扮演 Bot、陪伴型应用等场景。这类系统往往需要频繁调用第三方模型服务，并长期维护对话连续性、生成稳定性与用户体验一致性。

在实际开发中，一个普遍存在的问题逐渐显现：

- 不同模型服务的 API 规范并不统一
- 鉴权方式、参数结构、返回格式存在差异
- 服务行为随版本变动，给系统维护带来较大成本

为了解决这些问题，开发者通常会在业务系统与模型服务之间，引入一层 **统一的模型访问服务**，用于屏蔽底层差异、规范接口行为，并对生成过程进行记录与管理。

本轮考核正是基于这一真实而常见的工程背景：

> **设计并实现一个 OpenAI API 风格的后端服务，用于统一管理模型生成请求。**

请注意：

- 本考核 **不要求实现或训练大模型**
- 重点在于 **API 设计、服务语义与工程实现质量**

## 考核目标

通过本次考核，我们主要关注以下能力：

- 对 RESTful API 的系统性理解
- 对真实 SDK / 客户端使用方式的理解
- 对一次“模型生成请求”完整生命周期的建模能力
- 后端工程的结构设计、代码规范与可维护性
- 使用 Git 进行工程化开发与版本管理的能力

## 考核任务概述

请实现一个 **兼容（部分）OpenAI API 的后端服务**，支持通过 HTTP 接口提供模型推理相关能力。

生成内容可以：

- 使用 **固定模板或规则返回**（如 echo / mock / template），如果使用模板，请务必在返回内容中包含所有请求参数信息。
- 或 **转发至真实 LLM 服务**（如 OpenAI / Azure / 本地模型）

但无论采用哪种方式：

> **接口行为、参数结构与返回格式必须符合 OpenAI API 的基本语义**

## 必须完成的最小功能集（MVP）

以下内容为 **最低完成要求**，未完成将视为不合格。

### 1. 认证与鉴权

- **所有接口**都需要使用 Bearer Token 进行接口鉴权，以区分不同用户
- 未携带或非法 Token 的请求应返回 `401 Unauthorized`
- 鉴权逻辑应统一处理（如中间件 / 拦截器）

\*API 文档参考: [官方文档镜像站](https://ai-doc.it-docs.cn/api-reference_en/authentication)

### 2. 文本生成接口（核心） - Chat Completions

此接口用于处理对话生成请求，支持流式与非流式两种输出方式。

\*API 文档参考: [官方文档镜像站](https://ai-doc.it-docs.cn/api-reference_en/chat), [阿里云百炼通用千文 OpenAI 兼容文档](https://bailian.console.aliyun.com/cn-beijing/?tab=api#/api/?type=model&url=3016807)

#### Endpoint 示例

```
POST /v1/chat/completions
```

#### 基本要求

- 支持参数（至少）：
  - `model`
  - `messages`
  - `stream`
  - `temperature`（可忽略实际效果，但需解析）

##### 模型白名单（ai_models）

- `model` 必须存在于数据库表 `ai_models` 且 `enabled=1`，否则接口返回 `400 Bad Request`
- 目的：避免在代码中硬编码模型列表，通过数据库动态维护可用模型

示例：初始化/新增可用模型（MySQL）

```sql
INSERT INTO ai_models (model_id, owned_by, enabled, created)
VALUES ('MiniMax-M2.7', 'minimax', 1, UNIX_TIMESTAMP());
```

示例：当请求的 `model` 不在白名单时，返回错误体（OpenAI 风格）

```json
{
  "error": {
    "message": "Model not available",
    "type": "invalid_request_error"
  }
}
```

- 每一次生成请求：
  - 必须生成 **唯一 id**
  - 必须被服务端记录（使用数据库持久化）

- 返回格式需与 OpenAI API 保持基本一致

#### 流式/非流式输出支持

- 当 `stream=false` （默认值）：
  - 一次性返回完整生成结果

- 当 `stream=true`：
  - 使用 **服务端流式返回**（如 SSE / Chunked Response）
  - 不允许伪流式（一次性返回）

如果生成结果使用模板，请务必在每一次请求中添加必要的延迟（如 1 秒，这对于可选功能实现中的服务限流和最大并发数控制实现非常重要）

### 3. 生成结果管理

可参考 OpenAI Responses API 中对生成结果与生命周期的管理方式。使用数据库持久化每一次的请求响应信息，确保随时可访问。

- 获取某一次生成结果 `GET /v1/chat/completions/{completion_id}`
- 删除某一次生成结果 `DELETE /v1/chat/completions/{completion_id}`
- 取消正在进行的生成请求 `POST /v1/chat/completions/{completion_id}/cancel`

\*API 文档参考: [官方文档镜像站](https://ai-doc.it-docs.cn/api-reference_en/responses)

### 4. 模型列表

列出当前可用的模型，并提供每个模型的基本信息，例如所有者和可用性。

- 模型来源：数据库表 `ai_models`
- 仅返回 `enabled=1` 的模型；该列表同时作为 `POST /v1/chat/completions` 的 `model` 白名单来源

\*API 文档参考: [官方文档镜像站](https://ai-doc.it-docs.cn/api-reference_en/models)

#### Endpoint 示例

```
POST /v1/models
```

### 5. SDK 可用性验证（测试编写）

- 必须使用 **官方 OpenAI SDK** 编写测试脚本，以验证所实现的接口是否符合官方规范
- 测试脚本语言 **可与服务端实现语言不同**
- 测试需验证：
  - 接口可正常调用
  - 返回结果可被 SDK 正确解析

- （可选）添加覆盖度测试，测试的覆盖度应该达到 90% 以上
- （可选）使用 CI/CD 流水线在每次代码提交时都自动进行代码测试

## 可选功能（加分项）

以下功能不要求全部实现，请根据时间与能力自行选择：

完成其中两项即可获得全部分数。

### 1. Files API

可参考 OpenAI Files API 的资源模型与接口设计，用于管理模型生成或工具调用所需的文件资源。

- 文件上传 `POST /v1/files`
- 文件删除 `DELETE /v1/files/{file_id}`
- 文件列表查询 `GET /v1/files`
- 获取文件元信息 `GET /v1/files/{file_id}`

\*API 文档参考: [官方文档镜像站](https://ai-doc.it-docs.cn/api-reference_en/files), [阿里云百炼通用千文 OpenAI 兼容文档](https://bailian.console.aliyun.com/cn-beijing/?tab=api#/api/?type=model&url=2833610)

### 2. 系统与工程能力

如果你对后端技术感兴趣，那么非常推荐你实现其中的 1~2 个，它们都在实际服务搭建中非常常见：

- 服务限流（Rate Limit）：利用 Redis 或内存计数器等技术，按 API Key / 用户 / IP 进行限流，超出限制时返回明确错误码（如 429 Too Many Requests）
- 最大并发控制：利用任务队列或线程池等技术，控制同时进行的生成请求数量，并发超限时进行排队、拒绝或降级处理
- 上下文复用与生成去重：利用 Redis 或 LRU 内存管理技术，对短时间内的重复生成请求进行去重处理
- 安全性检查（参数校验、防滥用）：利用参数校验（Schema / 类型检查），在输入非法参数时返回明确错误信息，或防止异常输入（如 URL 注入）导致服务崩溃

### 3. Admin API

此部分 API 虽然不是标准 OpenAI API 实现，但可以帮助我们在前期快速初始化数据库等操作。

- 创建新的模型
- 添加、删除用户
- 给用户分配 API Key 、禁用某个 API Key
- 限制某个用户的用量（如最大并发数，每日限额）
- 查看每个用户的用量情况

### 4. API 中转

解决项目的原始痛点，解决各模型提供商 SDK API 实现并不统一的问题。

此部分难度较大且需要访问外网的能力，适合考核后想要将该考核内容发展为独立开源项目的同学。

- 支持将模型生成请求转发给真实的模型服务
- 至少支持两种不同的模型服务中转（比如 Google Gemini 和阿里百炼）
- 通过单独的配置文件或环境变量存储这些模型服务的鉴权密钥
- 支持不同粒度的限额管理，当上游模型提供商模型不可用时自动回退到其他可用的服务
- 实现 WebUI 以便于查看服务运行情况和配置管理

## 技术与工程要求

### 1. 通用要求

- 遵循 RESTful API 设计规范
- 使用 Swagger / Apifox 编写并导出 API 文档
- 统一的响应格式与异常处理
- 清晰的分层与模块划分（禁止所有逻辑堆在一层）

### 2. 代码规范

- 使用 UTF-8 编码
- 使用符合语言规范的命名方式
- 严禁使用拼音或无语义命名
- 代码中每一个函数类都需要写明清晰的函数文档（注意函数文档和代码注释的区别）

### 3. Git 与工程管理

- 必须使用 Git 进行版本控制
- 仓库需为 **公开仓库（不限制 Git 托管平台，比如 GitHub / Gitee）**
- 需要：
  - 合理的 commit 粒度
  - 规范的 commit message

- 鼓励使用 Github Project 或 Readme.md 中的一些章节记录设计、问题与计划（非强制，我个人不喜欢用 issue 记录开发计划，Github Project 对于进度管理来说更加规范）

## AIGC 使用说明

允许使用 AIGC 工具（如 ChatGPT、Github Copilot 等），但必须披露 AIGC 工具的使用情况：

- 在 README 或单独文档中说明：
  - 使用了哪些 AIGC 工具
  - 用于哪些部分（如鉴权、中间件、测试等）

> AIGC 使用 **不会直接影响评分**，但将作为面试提问的重要依据。

诚信是学术界的第一原则，请务必诚实撰写这一部分的内容。

## 项目交付物

请在仓库中至少包含以下内容：

1. 完整源代码（含注释） + 测试脚本
2. API 文档导出文件/预览地址（Swagger / Apifox）
3. README（Markdown），需包含：
   - 项目说明
   - 架构设计简述
   - 鉴权与生成流程说明
   - 快速启动方式
   - SDK 测试说明
   - AIGC 使用说明

## 可能的完整请求及响应

本节用于**帮助理解接口规范与语义**，不构成“照抄要求”，但实现应尽量保持行为一致。

### 1. Chat Completions

#### Endpoint

```
POST /v1/chat/completions
```

#### 请求示例

```
{
  "model": "gpt-4o-mini",
  "messages": [
    { "role": "system", "content": "You are a helpful assistant." },
    { "role": "user", "content": "Hello!" }
  ],
  "temperature": 0.7,
  "stream": false
}
```

#### 非流式响应示例

```
{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "created": 1710000000,
  "model": "gpt-4o-mini",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello! How can I help you today?"
      },
      "finish_reason": "stop"
    }
  ]
}
```

#### 流式响应语义说明

当 `stream=true` 时，服务端应以 **事件流** 的形式返回多个数据块，每个数据块表示一次增量输出。

示例（SSE 语义，简化）：

```
data: {"id":"chatcmpl-abc123","choices":[{"delta":{"content":"Hello"}}]}

data: {"choices":[{"delta":{"content":" world"}}]}

data: [DONE]
```

要求：

- 每个 chunk 必须是合法 JSON 或明确的结束标识
- 不允许一次性拼接后再返回

### 2. 生成结果管理

#### 获取生成结果

```
GET /v1/chat/completions/{completion_id}
```

示例响应

```json
{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "created": 1710000000,
  "model": "gpt-4o-mini",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello! How can I help you today?"
      },
      "finish_reason": "stop"
    }
  ]
}
```

#### 删除生成结果

```
DELETE /v1/chat/completions/{completion_id}
```

#### 取消生成请求

```
POST /v1/chat/completions/{completion_id}/cancel
```

### 3. Models

此接口用于列出当前可用的模型，并提供每个模型的基本信息，例如所有者和可用性。

#### Endpoint

```
GET /v1/models
```

#### 响应示例

```
{
  "object": "list",
  "data": [
    {
      "id": "gpt-4o-mini",
      "object": "model",
      "created": 1686935002,
      "owned_by": "organization"
    }
  ]
}
```

### 4. Files（可选）

此接口用于管理模型生成或工具调用所需的文件资源。

#### 上传文件

```
POST /v1/files
```

请求需使用 `multipart/form-data`，字段示例：

- `file`: 上传的文件
- `purpose`: "assistants" / "fine-tune"（可忽略语义，仅做透传）

### 5. 错误响应规范

当请求非法或鉴权失败时，应返回结构化错误信息。

#### 示例（401）

```
{
  "error": {
    "message": "Invalid API key",
    "type": "authentication_error"
  }
}
```

## 参考资源

以下资源可用于理解接口行为与 SDK 使用方式：

- [OpenAI API Reference (需要科学上网)](https://developers.openai.com/api/reference/overview)
- [文档镜像站(部分内容可能已经过时，无需科学上网)](https://ai-doc.it-docs.cn/api-reference_en/introduction)
- [阿里云百炼通用千文 OpenAI 兼容文档](https://bailian.console.aliyun.com/cn-beijing/?tab=api#/api/?type=model&url=3016807)

以下是根据官方接口实现搭建的示例服务，可用于参考接口行为与测试 SDK 使用：

- [示例服务](http://8.134.203.128:8000/v1)
- [示例服务的 API 文档](http://8.134.203.128:8000/docs)
- [示例服务的 openapi.json](http://8.134.203.128:8000/openapi.json)
- [参考 Python 实现](https://github.com/Moemu/2025-backend-recruit-03-openai/tree/main/Examples/Python)

> 提示：请重点关注 **参数结构、返回格式与流式语义**，而非具体模型能力。

祝你编码顺利。
