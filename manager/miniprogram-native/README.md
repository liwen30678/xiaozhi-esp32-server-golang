# 原生小程序控制台（MVP）

这是基于现有 `manager/backend` API 的原生微信小程序控制台示例，不依赖 web-view。

## 已实现页面

- 登录：`pages/login`
- 控制台首页：`pages/console`
- 智能体列表：`pages/agents`（含新增、删除）
- 智能体编辑：`pages/agent-edit`
- 智能体设备管理：`pages/agent-devices`
- 智能体聊天历史：`pages/agent-history`
- 设备列表：`pages/devices`（含设备角色应用）
- 我的角色：`pages/roles`
- 我的知识库：`pages/knowledge-bases`
- 声纹管理：`pages/speakers`（含样本管理、验证）
- 声音复刻：`pages/voice-clones`（含创建、试听、重试、改名）
- 我的信息/退出：`pages/profile`

## 对接接口

- `POST /api/login`
- `GET /api/profile`
- `GET /api/dashboard/stats`
- `GET /api/user/agents`
- `POST /api/user/agents`
- `PUT /api/user/agents/:id`
- `DELETE /api/user/agents/:id`
- `GET /api/user/agents/:id/devices`
- `POST /api/user/agents/:id/devices`
- `DELETE /api/user/agents/:id/devices/:device_id`
- `GET /api/user/devices`
- `POST /api/user/devices`
- `POST /api/user/devices/inject-message`
- `POST /api/devices/:id/apply-role`
- `GET /api/user/roles`
- `POST /api/user/roles`
- `PUT /api/user/roles/:id`
- `PATCH /api/user/roles/:id/toggle`
- `DELETE /api/user/roles/:id`
- `GET /api/user/knowledge-bases`
- `POST /api/user/knowledge-bases`
- `PUT /api/user/knowledge-bases/:id`
- `DELETE /api/user/knowledge-bases/:id`
- `POST /api/user/knowledge-bases/:id/sync`
- `GET /api/user/history/agents/:id/messages`
- `GET /api/user/speaker-groups`
- `POST /api/user/speaker-groups`
- `PUT /api/user/speaker-groups/:id`
- `DELETE /api/user/speaker-groups/:id`
- `GET /api/user/speaker-groups/:id/samples`
- `POST /api/user/speaker-groups/:id/samples`
- `DELETE /api/user/speaker-groups/:id/samples/:sample_id`
- `POST /api/user/speaker-groups/:id/verify`
- `GET /api/user/voice-clone/capabilities`
- `GET /api/user/voice-clones`
- `POST /api/user/voice-clones`
- `PUT /api/user/voice-clones/:id`
- `POST /api/user/voice-clones/:id/retry`
- `POST /api/user/voice-clones/:id/append-audio`
- `GET /api/user/voice-clones/:id/preview`
- `GET /api/user/voice-clones/:id/audios`
- `GET /api/user/voice-clones/audios/:audio_id/file`

## 使用方式

1. 在微信开发者工具中导入 `manager/miniprogram-native`。
2. 登录页填入后端地址（例如 `https://your-manager-domain.com`）和账号密码。
3. 登录成功后进入控制台 tab。

## 本地联调提示

- 如需使用内网地址（如 `http://192.168.x.x:8080`）调试，请在开发者工具中关闭“校验合法域名”。
- 真机/预览/线上环境必须使用已在小程序后台配置的 `HTTPS` 合法域名，且不能使用 IP 直连。

## 说明

- 当前采用 Bearer Token（`Authorization`）鉴权。
- 若你需要“管理员配置管理”等高级页面，可在此基础上继续扩展。
- 生产环境请在小程序后台配置合法域名，且后端必须 HTTPS。
