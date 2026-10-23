# 本地与生产部署

## 构建产物

后端由 Go 标准库 HTTP 托管 `web/dist`，SPA 深层路由刷新可用；未知 `/api` 路径返回 JSON 404，不回退前端 HTML。前端未构建时需先运行 `npm ci && npm run build`，不能把开发服务器当生产入口。

```sh
cd web
npm ci
npm run build
cd ..
go build -trimpath -o controller ./cmd/controller
./controller -config config.local.json
```

Linux/macOS 可手动复制 `config.example.json` 为 `config.local.json`，将两个密钥分别替换为独立的随机值；例如在本机用 `openssl rand -hex 32` 生成，勿将生产凭据发送给第三方。限制配置权限（如 `chmod 600 config.local.json`）。配置文件不需要打包进镜像。

## Compose

Dockerfile 的前两阶段分别构建前端和 Go 二进制；最终为无 shell 的 distroless 非 root 镜像。`CGO_ENABLED=0` 配合纯 Go SQLite 驱动，无需在运行镜像内安装编译器或 Node。

可选 `backend-test` 阶段使用隔离的C编译器执行Go竞态检测：`docker build --target backend-test .`。该阶段不属于最终运行镜像的依赖链。

1. 从 `.env.example` 创建 `.env`，设置两个独立随机密钥。
2. 设置实际访问的 `CONTROLLER_PUBLIC_ORIGIN`；HTTPS必须搭配 `CONTROLLER_SECURE_COOKIE=true`，HTTP本机开发必须为false，配置不一致将拒绝启动。
3. 将实际服务精确 origin 加入 `CONTROLLER_ALLOWED_SERVICE_ORIGINS`。
4. 按 `service-access.example.json` 准备 `secrets/service-access.json`，配置每服务独立业务令牌和精确接口白名单，并在`.env`设置`CONTROLLER_SERVICE_ACCESS_FILE=/run/secrets/service-access.json`。服务自身设置相同业务令牌与中心来源IP白名单。
5. `docker compose up -d --build`。
6. 打开界面初始化管理员。初始化、注册、业务访问三类密钥职责独立，不能复用。

Compose 使用固定项目名 `workstation-controller`，避免中文项目目录被Docker转换为空名称。服务使用非特权用户、只读根目录、丢弃所有 Linux capabilities，并禁止提升权限。只有数据卷与小型 `/tmp` 可写。默认仅暴露主机回环端口，是否增加公网/局域网入口由部署者决定。

`/healthz` 返回基本健康状态，不泄漏数据库路径/密钥/管理员数据。镜像内置 `/app/controller -healthcheck`，Compose 可据此展示健康状态。

## HTTPS 与服务网络

可在中心前方部署由用户管理的HTTPS入口，转发至中心8080，保留标准Host/Origin，设置访问控制与请求大小限制。中心另提供受限业务JSON网关；它不接受任意URL，也不透传浏览器认证头。不要把不可信转发头当认证身份。

独立服务业务请求由中心发起，必须满足：

- `base_url` 的 origin 在中心白名单中。
- 仅中心进程需要访问该地址；浏览器不连接业务端口。
- 部署策略固定服务id、origin、接口方法与路径，并配置独立随机业务Bearer令牌；服务校验该令牌与中心真实出口IP，不为浏览器开放CORS。
- 非回环连接必须HTTPS并验证证书；HTTP只用于实际本机回环。容器跨网络接入应使用正确域名、证书和防火墙规则。
- 服务自行控制认证、网络防火墙及反向代理，不要将内网地址自动发布到公网。

业务令牌通过只读策略文件提供，不放入前端、SQLite登记或镜像，不透传Cookie或注册密钥。示例服务默认回环监听，要求业务令牌与IP白名单。服务级令牌不等于完整业务权限模型；生产服务应给中心配置最小权限账号。配置示例、调用协议与限制统一由后台可开关的GET /api/v1/integration/handbook提供；默认关闭，开启后只公开通用文档，不公开当前配置。

## 数据与备份

SQLite 数据目录默认为 `data/`，Compose 中为 `/app/data` 的 `controller-data` 命名卷。数据库包括管理员哈希、会话哈希、登记信息、定义、状态、系统设置、事件和审计，应像敏感配置一样保护。

最简单可靠的备份方式：停止中心实例，复制整个数据目录，再启动中心。使用 WAL 时不要只复制正在写入的 `.db` 而遗漏相关文件。备份目录同样包含敏感认证信息，应加密存储并限制权限。恢复前先停止中心，保留现有数据副本，然后恢复匹配版本的数据。

升级时先备份，再重建镜像并 `docker compose up -d`；迁移版本存于数据库，应用启动时运行尚未执行的迁移。不要运行多个实例共同写同一个 SQLite 文件，不承诺自动降级迁移。**`docker compose down -v` 会删除命名卷，不是常规升级步骤。**

## 运维

- `docker compose logs --tail=100 controller` 查看结构化日志。
- `docker compose ps` 查看进程/健康状态。
- 后台可修改站点名称和检查间隔；安全白名单和注册密钥通过部署配置修改后重启。
- 注册密钥轮换后需更新各服务的注册配置；现存登记保留，无法重新注册不会自动删除。
- 健康检查超时、DNS或连接错误只更新对应服务在线状态，不影响管理接口。
- 删除登记不会停止服务；仍在注册的服务会再次出现为待启用。
- 忘记管理员密码时第一版没有在线重置接口。不要直接手改密码字段或清空生产数据库；先完整备份并走受控维护流程。
