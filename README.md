# Server Manager

轻量级服务器与网站管理平台 —— 单容器、低资源、Go + React + SQLite，无 Chromium、无 Redis、无 PostgreSQL。

管理 SSH 服务器、Web Terminal、命令模板、Docker 容器，以及 OpenList / OpenWrt / NAS / 路由器等 Web 服务的入口与凭据。

---

## 快速开始

### Docker（推荐）

```bash
docker compose up -d
# 打开 http://localhost:8080
# 默认账号 admin / please-change-me（见 docker-compose.yml）
```

或单条命令：

```bash
docker run -d --name server-manager -p 8080:8080 \
  -v ./data:/data \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -e APP_ADMIN_PASSWORD=please-change-me \
  server-manager:1.0.0
```

### 本地开发

```bash
# 后端
go run ./cmd/server                 # 监听 :8080

# 前端（另一个终端，vite dev server :5173，已配置 /api 代理）
cd web && npm install && npm run dev
```

### 构建产物

```bash
cd web && npm run build             # -> web/dist
go build -o bin/server-manager ./cmd/server   # dist 被 go:embed 打进二进制
```

生产运行时只有一个静态二进制 + SQLite 文件，容器内不含 Node / Go 工具链。

### 交叉编译

前端 `web/dist` 存在时可直接产出其他平台的二进制（`go:embed` 会打包同一份前端）：

```bash
CGO_ENABLED=0 GOOS=linux  GOARCH=amd64 go build -o bin/server-manager-linux ./cmd/server
CGO_ENABLED=0 GOOS=linux  GOARCH=arm64 go build -o bin/server-manager-linux-arm64 ./cmd/server
```

---

## 开发与测试辅助（无真机也能验证）

`tools/` 下两个假服务，用于在没有真实环境时端到端验证链路，不属于运行时依赖：

```bash
# 假 SSH 服务器：127.0.0.1:2222，账号 test / test
go run ./tools/fake-ssh serve

# 假 SSH 之上再验证 WebSocket 终端（SM_COOKIE 为登录后 sm_session 的值）
SM_COOKIE=<session> go run ./tools/fake-ssh ws "ws://127.0.0.1:8080/api/servers/<id>/terminal"

# 假 Docker Engine：仅实现 /containers|images|volumes|networks|logs
python tools/fake-docker.py 2375
DOCKER_HOST=tcp://127.0.0.1:2375 go run ./cmd/server
```

---

## 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `APP_ENV` | `dev` | `prod` 时启用 Secure cookie；非 prod 放开 localhost:5173 跨域便于开发 |
| `APP_PORT` | `8080` | 监听端口 |
| `APP_DATA_DIR` | `./data` | 数据目录（自动生成的加密主密钥落在这里） |
| `APP_DATABASE` | `$APP_DATA_DIR/app.db` | SQLite 文件路径 |
| `APP_ENCRYPTION_KEY` | 空 | 凭据加密主密钥；留空会生成随机密钥写入 `$APP_DATA_DIR/.encryption_key`（**务必备份该文件**） |
| `APP_SESSION_SECRET` | 空 | 保留给后续签名场景 |
| `APP_ADMIN_USERNAME` / `APP_ADMIN_PASSWORD` | `admin` / `admin123` | 首次启动自动创建管理员，之后可在「设置 → 账号」改密码 |
| `DOCKER_HOST` | `unix:///var/run/docker.sock` | Docker Engine 地址，支持 `npipe://`（Windows）与 `tcp://` |

---

## 功能

| 模块 | V1 已实现 |
| --- | --- |
| Dashboard | 服务器数、在线数、网站数、命令数；最近操作（在线判定为并发 TCP 探活，约 2s 内返回） |
| Servers | CRUD、凭据保存、连通性测试、Web Terminal |
| Terminal | xterm.js + WebSocket + SSH PTY，支持 ANSI 颜色、resize、Ctrl+C/D、方向键历史、Ctrl+Shift+C/V 复制粘贴、Ctrl+Shift+F 全屏、断线提示与重连 |
| Commands | 模板 CRUD、`{{param}}` 参数、风险分级、高危二次确认、在指定服务器执行并记录日志 |
| Websites | CRUD、分类、图标、URL、凭据加密保存、新标签页打开 |
| Docker | 容器列表 / 启停 / 重启 / 删除 / 日志 / inspect；镜像、数据卷、网络 |
| Settings | 账号改密、系统信息、凭据元数据 |
| Activity | 登录、服务器/命令/网站/容器操作审计日志 |

### Terminal WebSocket 协议

```
GET /api/servers/{id}/terminal   （需登录 cookie）

Client → Server:  {"type":"input","data":"ls -la\n"}
                  {"type":"resize","cols":120,"rows":40}
Server → Client:  {"type":"output","data":"..."}
                  {"type":"error","message":"..."}
                  {"type":"closed"}
```

### API 一览

```
POST   /api/auth/login                POST   /api/auth/logout
GET    /api/auth/me                   PUT    /api/auth/password
GET    /api/dashboard/stats           GET    /api/activity
GET    /api/system/info

GET    /api/servers                   POST   /api/servers
GET    /api/servers/{id}              PUT    /api/servers/{id}
DELETE /api/servers/{id}              POST   /api/servers/{id}/test
GET    /api/servers/{id}/terminal     (WebSocket)

GET    /api/commands                  POST   /api/commands
PUT    /api/commands/{id}             DELETE /api/commands/{id}
POST   /api/commands/{id}/render
POST   /api/servers/{serverId}/commands/{commandId}/execute

GET    /api/websites                  POST   /api/websites
GET    /api/websites/{id}             PUT    /api/websites/{id}
DELETE /api/websites/{id}

GET    /api/credentials               POST   /api/credentials
PUT    /api/credentials/{id}          DELETE /api/credentials/{id}

GET    /api/docker/{containers,images,volumes,networks,status}
POST   /api/docker/containers/{id}/{start,stop,restart}
DELETE /api/docker/containers/{id}
GET    /api/docker/containers/{id}/{logs,inspect}
```

---

## 安全设计

- **密码**：Argon2id（`m=64MB, t=2, p=2`）存储，绝不明文。
- **凭据**：统一 `credentials` 表，AES-256-GCM 加密；SSH 密码、私钥、网站密码/Token/Cookie 全部密文落盘。
- **主密钥来源**：环境变量优先，否则读 `$APP_DATA_DIR/.encryption_key`（首次自动生成，600 权限）。
- **API 不返回敏感值**：列表/详情只给 `credential_id` 与凭据名称，密文不出库、不出接口。
- **命令注入防护**：`{{param}}` 参数白名单校验（仅 `字母数字 . _ @ : % / , -`），不合法直接拒绝。
- **高危命令**：`high` / `critical` 必须二次确认；后端还会对渲染后的命令做静态分析（rm -rf、mkfs、dd、iptables、docker system prune…）自动提级。未确认时返回 `409 CONFIRMATION_REQUIRED`。
- **会话**：随机 32 字节 token，HttpOnly + SameSite=Lax cookie，prod 下加 Secure；登录失败 5 次锁定 15s/IP。
- **CSRF**：非 GET 请求校验 Origin 同源。
- **审计**：Activity Log 只记录操作名与目标，不含任何密码/Token/私钥。
- **边界**：浏览器永远不直连 SSH 或 docker.sock，一切经后端代理；生产环境建议反代 HTTPS。

---

## 目录结构

```
server-manager/
├── cmd/server/main.go            入口
├── internal/
│   ├── api/                      HTTP/WebSocket 层（chi）
│   ├── auth/                     Argon2id + 会话
│   ├── config/                   环境变量配置
│   ├── credential/               AES-256-GCM 加解密与凭据仓储
│   ├── database/                 SQLite 连接 + migrations/*.sql（embed）
│   ├── sshclient/                SSH 拨号、单命令执行、PTY 会话
│   ├── commandexec/              模板渲染、参数校验、危险等级分析
│   ├── dockerapi/                Docker Engine REST API 客户端
│   └── activity/                 审计日志
├── web/                          React + TS + Vite + Ant Design + xterm.js
├── tools/                        fake-ssh / fake-docker，无真机时的链路验证工具
├── .github/workflows/image.yml   跨平台编译校验 + 多架构镜像构建（GHCR）
├── Dockerfile                    三阶段构建，运行镜像仅含静态二进制
├── docker-compose.yml
└── README.md
```

---

## V1 范围之外

按设计刻意不做：Chromium / Playwright / 浏览器自动化、iframe 代理、网站自动登录、WebRTC、Kubernetes、微服务、消息队列、复杂 RBAC。这些都留给 V2/V3 的独立 Browser Service 或 Authentication Adapter。

---

## 已知取舍

- Session 保存在进程内存中，服务重启后需要重新登录（单管理员场景可接受）。
- SSH HostKey 采用首次信任策略（个人工具场景），未实现 known_hosts 校验。
- 网站模块 V1 只加密保存凭据 + 点击打开，不保证自动登录 —— 各站点登录机制差异过大，认证适配层留给后续版本。
