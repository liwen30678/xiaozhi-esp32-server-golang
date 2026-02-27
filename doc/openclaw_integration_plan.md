# OpenClaw 接入优化方案（已固化参数版）

> 目标：设备绑定智能体后，默认走智能体 LLM；用户可通过自定义“进入/退出”关键词切换到 OpenClaw。OpenClaw 采用被动连接（按需首连），并具备全局会话管理与离线消息回投能力。

---

## 1. 已确认的产品/策略决策

1. **连接策略**：被动连接，首次实际需要调用 OpenClaw 时才建立连接。
2. **离线暂存位置**：存储在 `manager/backend`（持久化到 manager 数据库）。
3. **离线消息 TTL**：不设置过期（无限制）。
4. **离线队列上限**：每设备最多 100 条（FIFO 丢弃最旧）。
5. **上线回投方式**：方式 B（设备上线后主动补发离线消息）。
6. **失败回退策略**：OpenClaw 连续 3 次失败自动回到 LLM 模式。

---

## 2. 总体架构

## 2.1 会话双路由

- `llm_router`：默认路由，沿用现有 ASR -> LLM -> TTS。
- `openclaw_router`：进入 OpenClaw 模式后路由到 openclaw-go 客户端。

根据 `session_mode` 决定路由：
- `session_mode=llm` → 走原链路
- `session_mode=openclaw` → 走 OpenClaw

## 2.2 两个新核心组件

1. **OpenClaw 全局连接会话管理器**（主程序）
   - 负责按需建连、连接复用、并发控制、失败计数与回退触发。
2. **OpenClaw 离线消息池**（manager/backend）
   - 负责持久化长任务迟到回复，等待设备上线后补发。

---

## 3. 数据模型设计

## 3.1 主程序运行态（内存）

- `device_session_mode[device_id] = llm/openclaw`
- `openclaw_session_pool[session_key]`
  - `session_key = user_id + agent_id + openclaw_config_id`
  - 包含连接状态、失败计数、最后活跃时间、挂起任务映射

## 3.2 manager/backend 持久化表（新增）

建议新增表：`openclaw_offline_messages`

字段建议：
- `id`
- `device_id`（索引）
- `user_id`（索引）
- `agent_id`（索引）
- `openclaw_config_id`
- `task_id`（唯一约束，防重复）
- `message_type`（text/json/event）
- `payload_json`
- `status`（pending/delivered/failed）
- `retry_count`
- `delivered_at`
- `created_at`, `updated_at`

约束：
- 每设备仅保留最近 100 条 pending（超过则删最旧）。
- TTL 不做自动过期清理。

---

## 4. 端到端流程（配置 -> 连接 -> 消息处理）

## 4.1 配置阶段

1. 用户在 manager/backend 维护 OpenClaw 网关配置。
2. 用户将配置绑定到 Agent，并设置自定义进入/退出关键词。
3. 主程序通过 `/api/configs` 拉取设备配置，拿到 OpenClaw 字段。

## 4.2 首次进入 OpenClaw（被动连接）

1. 会话收到文本后，先做 `mode command detector`（支持自定义关键词）。
2. 命中进入关键词：`session_mode` 切到 `openclaw`，返回确认语。
3. 下一条实际业务消息触发 OpenClaw 调用：
   - 先从全局会话管理器查连接
   - 不存在则建连（Lazy Connect）
   - 发送请求并等待响应

## 4.3 长任务与迟到响应

1. 若 OpenClaw 是长任务，先返回 `task_id` 并进入挂起态。
2. 后续回调/轮询拿到结果时：
   - 若设备在线：实时下发
   - 若设备离线：写入 `openclaw_offline_messages` pending

## 4.4 设备上线回投（方式 B）

1. 设备新连接建立后（OnNewConnection 成功后），触发离线回投流程。
2. 从 manager/backend 拉取该设备 pending 消息（按创建时间升序）。
3. 逐条注入会话回复并标记 delivered。
4. 单次回投失败保留 pending，后续设备再次上线继续补发。

---

## 5. 失败处理与回退

## 5.1 连续失败回退

- 同一设备处于 openclaw 模式时，调用失败计数 `fail_count`。
- `fail_count >= 3`：
  1) 自动切回 `session_mode=llm`
  2) 给用户播报“OpenClaw 暂不可用，已回到默认助手”
  3) 清理当前 OpenClaw 会话上下文（不删离线池）

## 5.2 幂等与去重

- 使用 `task_id` 做唯一去重，防止回调重复写入/重复回投。

---

## 6. 模块落地建议

## 6.1 主程序新增目录

`internal/domain/openclaw/`
- `session_manager.go`：全局连接会话池、懒连接、失败计数
- `client.go`：openclaw-go 封装
- `router.go`：session_mode 路由
- `dispatcher.go`：在线下发 or 离线落库

## 6.2 manager/backend 新增能力

- 模型：`OpenClawOfflineMessage`
- 接口（内部）：
  - `POST /api/internal/openclaw/offline-messages`
  - `GET /api/internal/openclaw/offline-messages?device_id=...&status=pending`
  - `POST /api/internal/openclaw/offline-messages/:id/delivered`
- 队列上限维护：写入前按设备裁剪到 100 条。

---

## 7. 实施分期（按风险）

### Phase A（先打通）
- 主程序 session_mode 路由 + 关键词切换
- openclaw-go 懒连接与基础调用
- manager/backend 离线消息表 + 内部接口

### Phase B（长任务）
- task_id 挂起管理
- 迟到响应离线入库 + 在线直发分流

### Phase C（回投与稳态）
- 设备上线自动回投（方式 B）
- 失败重试、幂等、指标告警

---

## 8. 验收标准（本次确认后按此交付）

1. 默认仍为 LLM 模式，进入关键词后才转 OpenClaw。
2. OpenClaw 首次调用前不建连（被动连接成立）。
3. 设备离线时，OpenClaw 长任务回复可落到 manager/backend。
4. 设备上线后可自动补发离线消息（方式 B）。
5. 每设备离线消息最多 100 条，TTL 无限制。
6. OpenClaw 连续 3 次失败自动回到 LLM。

---

确认后我将按 Phase A 开始改代码（先主程序懒连接 + manager/backend 离线池基础表与内部接口）。
