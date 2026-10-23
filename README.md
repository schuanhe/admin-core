# 个人工作站中心控制器

一个个人自用的轻量控制台。中心维护管理员、独立服务登记、启用状态、健康检查、菜单与页面 JSON；Vue 使用自身受限组件渲染页面。中心通过**管理员鉴权与部署白名单控制的受限业务网关**访问服务接口；不提供任意 URL 转发、不操作 Docker、不接管独立服务的数据或部署。

## 已实现

- 管理员一次性初始化、登录、带有效期的 HttpOnly 会话、CSRF 防护、密码哈希、登录限速和操作审计。
- 主动注册与自动登记、注册认证、重复注册更新、新服务待启用、搜索和状态筛选、详情、启用/禁用、离线删除、手动/定时健康检查。
- `pending / enabled / disabled` 与 `unknown / online / offline` 独立建模，服务恢复不丢管理员选择。
- SQLite 持久化及版本化迁移，系统设置、注册事件、最近错误与审计记录。
- Vue 3 + TypeScript + Naive UI 中文管理后台；白名单 `table / form / detail / compose` 页面及动态菜单。
- 组合页面支持栅格、堆叠、卡片、页签、文本、提示、统计、进度、描述列表、分隔线、列表、页面链接及嵌入页面；表单支持多行文本、单选、日期和输入范围。
- 服务可直接提供CSS或声明式style，自动限定页面作用域，不污染侧栏及其他服务，拒绝外部资源和危险样式能力。
- 服务菜单分组折叠、内置图标配置、站点/服务/页面面包屑及可配置的本服务页面层级。
- 服务导航拖动排序与固定置顶，数据库保存偏好；重新登记和中心重启均保留。
- 后台可开关的AI接入手册接口；组件、CSS、图标目录与实际白名单共用代码注册表，支持Markdown和JSON。
- 服务业务令牌与来源IP双重校验，受限网关仅调用页面定义与部署策略的交集，密钥不返回浏览器。
- 独立示例服务、生产多阶段镜像与 Compose；本次网关变更按要求未运行测试。

## 本地快速开始（Windows PowerShell）

需要 Go 1.27.3（较旧 Go 可通过 `GOTOOLCHAIN=auto` 自动下载工具链）和 Node.js 24。网络需能访问 Go 模块源和 npm。

如果 `config.local.json` 和 `.env` 已存在，请直接使用已有配置，跳过下面的第1步；初始化脚本会拒绝覆盖它们。

```powershell
# 1. 在项目根目录生成独立随机密钥，不覆盖已有配置
powershell -ExecutionPolicy Bypass -File .\scripts\init-dev.ps1

# 2. 安装前端依赖（已安装可跳过）
cd web
npm ci
cd ..

# 3. 使用开发模式启动前端、后端与独立示例服务
# 脚本只补充缺失的业务策略，使用 npm run dev 和 go run，不打包、不测试
powershell -ExecutionPolicy Bypass -File .\scripts\start-local.ps1
```

打开 [http://127.0.0.1:5173](http://127.0.0.1:5173)。首次访问进入初始化页：填写 `config.local.json` 中的 `bootstrap_token`，自行设置用户名和至少12字符的密码。初始化只允许成功一次，随后使用刚设置的管理员登录。开发时只从5173访问界面，8080保留为后端API端口，避免读到旧的前端构建产物。

示例服务监听 `127.0.0.1:8091`，立即主动注册，并每30秒重试/更新登记。在“服务管理”里启用“工作站示例服务”并检查状态，侧栏会出现“工作台”“任务列表”“偏好设置”“服务概览”。工作台展示真实统计、进度、页签及嵌入页面，并演示页面作用域CSS。归档和表单保存会修改该进程内存；重启示例服务会重置其演示数据。

停止示例服务后点击“重新检测”，记录转为离线，中心仍可正常使用，也可禁用或删除。再次启动示例服务并重新检测即可恢复；如删除后服务仍在周期注册，记录会重新出现并回到待启用，这是主动发现的预期行为。要持续隐藏它，请禁用记录或停止服务注册。

`config.local.json`、`.env`、`secrets/` 和 `data/` 都被 Git 和镜像构建忽略。不要提交、截图分享或上传本地密钥文件。

## 开发模式

Windows 可使用一条命令在后台启动前端、后端和独立示例服务（只用 npm run dev / go run，不打包、不测试、不覆盖本地配置）：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-local.ps1
```

打开 [http://127.0.0.1:5173](http://127.0.0.1:5173)。顶部可切换白天／黑夜；统一使用 #3478F6 / #6AA8FF / #D9EBFF 蓝白配色。默认白色界面，外观偏好仅保存在当前浏览器，不会修改独立服务的业务设置。启动日志保存在 `.runtime/local-*`，端口占用时脚本拒绝启动，不会关闭现有进程。

前端修改通过Vite即时更新；Go源码修改后需重启对应的 go run 进程。服务分组可点击折叠，用左侧拖动柄上下调整顺序，右侧图钉固定到顶部；固定与普通服务分组内分别排序。聚焦拖动柄后也可用Alt+↑/↓移动。顺序与固定状态保存到数据库，不随服务重新注册改变。

如需在不同终端分别运行：

停掉之前的后端进程，开发时使用与 Vite 一致的公开来源重新启动：

```powershell
$env:CONTROLLER_PUBLIC_ORIGIN = 'http://127.0.0.1:5173'
$env:CONTROLLER_SERVICE_ACCESS_FILE = 'secrets/service-access.json'
go run ./cmd/controller -config config.local.json
```

另开终端运行前端：

```powershell
cd web
npm run dev
```

访问 [http://127.0.0.1:5173](http://127.0.0.1:5173)。来源必须完全一致，否则后端会拒绝登录/初始化/写操作；不要混用 localhost 与 127.0.0.1。返回 Go 托管模式时移除该环境变量（`Remove-Item Env:CONTROLLER_PUBLIC_ORIGIN`），恢复配置中的 `http://localhost:8080`。Vite 仅将中心 `/api` 请求转发到本机后端。浏览器业务请求也通过中心鉴权网关，无法提交任意URL或选择任意接口。独立服务不再向浏览器开放CORS。开发服务仅绑定本机，勿对公网暴露。

## 配置

启动参数 `-config 路径` 加载 JSON 文件；环境变量覆盖文件；未配置项使用默认值。Go 不会隐式读取 `.env`，`.env` 用于 Docker Compose。可参考 [config.example.json](config.example.json) 和 [.env.example](.env.example)。

| 环境变量 | 默认值 / 用途 |
| --- | --- |
| `CONTROLLER_LISTEN` | `127.0.0.1:8080` |
| `CONTROLLER_DATABASE_PATH` | `data/controller.db` |
| `CONTROLLER_STATIC_DIR` | `web/dist` |
| `CONTROLLER_PUBLIC_ORIGIN` | `http://localhost:8080`；浏览器访问的精确来源 |
| `CONTROLLER_BOOTSTRAP_TOKEN` | 必填；初始化密钥，至少32随机字符 |
| `CONTROLLER_REGISTRATION_KEY` | 必填；与初始化密钥不同，至少32随机字符 |
| `CONTROLLER_ALLOWED_SERVICE_ORIGINS` | `http://127.0.0.1:8091`；逗号分隔、精确 scheme/host/port |
| `CONTROLLER_SERVICE_ACCESS_FILE` | 独立业务访问策略文件；未配置时业务网关默认拒绝 |
| `CONTROLLER_SESSION_HOURS` | `12` |
| `CONTROLLER_SECURE_COOKIE` | `false`；HTTPS生产环境须为 `true` |
| `CONTROLLER_CHECK_INTERVAL_SECONDS` | `30`；未保存系统设置时的默认值，之后以后台保存值为准 |

健康检查白名单属于部署安全边界，只能修改配置后重启；后台仅显示，不提供任意地址探测。站点名称和检查间隔在后台修改并持久化。避免使用共享注册密钥接纳不受信任的第三方服务。

## 测试和构建

以下命令供后续自行验收。本次受限网关与品牌配色修改**未运行测试**；旧测试记录不能视为本次验证结果。

```powershell
go test ./...
go vet ./...
go build ./cmd/controller
go build ./cmd/demo-service
cd web
npm test
npm run build
cd ..
node scripts/integration.mjs

# 可选：在隔离环境执行Go竞态检测，以及完整容器验收
docker build --target backend-test .
powershell -ExecutionPolicy Bypass -File .\scripts\docker-smoke.ps1
```

集成脚本自动编译、随机选取本地端口、生成临时密钥、启动中心和独立服务、执行完整验收并结束子进程。结果写入 `.runtime/integration-*/result.json`，临时数据库与配置保留便于排错，含随机测试凭据，请勿共享。它不使用 `data/controller.db`，不删除已有数据。测试结果见 [docs/testing.md](docs/testing.md)。

容器验收需先执行 `docker build -t workstation-controller:local .`。脚本使用独立随机Compose项目名和临时卷，验证完成后只清理本次测试项目；不会清理正式数据卷。

## 生产部署

详见 [docs/deployment.md](docs/deployment.md)。最简命令：

```powershell
# 先准备 .env 的真实随机密钥、HTTPS来源及允许的服务地址
docker compose up -d --build
docker compose ps
```

Compose 默认只绑定 `127.0.0.1:8080`，数据存储在 `controller-data` 命名卷。生产应使用自己管理的 HTTPS 入口，设置 `CONTROLLER_SECURE_COOKIE=true`。镜像使用非 root 用户和只读根文件系统，不包含密钥。

**容器内的 `127.0.0.1` 是容器自身。** 接入实际服务时，应填写仅中心可达的实际HTTPS地址，并配置中心的origin／接口白名单及服务的业务令牌／来源IP白名单；不能直接把本机开发示例地址照搬进生产容器。

## 项目边界和扩展

- `cmd/controller`：进程入口与生命周期。
- `internal/config`：配置解析及安全约束。
- `internal/store`：SQLite 数据访问与嵌入式迁移。
- `internal/auth`：密码、会话与认证。
- `internal/protocol`：服务/页面协议验证。
- `internal/registry`：注册管理与受限健康检查。
- `internal/httpapi`：HTTP 路由、鉴权、错误处理与静态页面。
- `web/src`：管理界面；`web/src/dynamic`：独立的定义验证、数据访问和受限渲染器。
- `cmd/demo-service`：不依赖中心业务代码的独立示例服务。

新增普通服务按中心接入手册实现注册和页面JSON，配置网络与接口白名单、独立业务令牌并主动注册，无需修改中心业务代码。新增“组件种类”仍需在中心实现渲染及语义校验，并在共享注册表声明字段与说明；手册自动跟随，不另维护一套组件名单。不支持任意JavaScript插件。

### 给其他AI的接入手册接口

在“系统设置 → 服务接入手册”开启并保存，然后复制接口地址给其他AI：

- [Markdown完整手册](http://127.0.0.1:5173/api/v1/integration/handbook)
- [JSON契约及完整说明](http://127.0.0.1:5173/api/v1/integration/handbook?format=json)

这是同一个GET接口的两种格式；直接访问后端8080同路径也可以。默认关闭，关闭后两种格式均返回404，重启保持设置。开启后无需管理员会话即可读取通用手册，**不返回密钥、已登记服务、实际地址或部署策略**；不要为此公开Vite端口。外部AI无法访问本机127.0.0.1时，应由本机工具读取并提供内容，或自行配置安全的可达入口，不自动暴露业务接口。

接口包含注册认证、独立服务接入步骤、网关与业务响应契约、菜单/面包屑/图标、页面/组件/表单、CSS作用域与白名单、资源限制、完整注册示例和占位部署策略。组件字段与CSS名单直接来自校验器使用的 [共享注册表](web/src/dynamic/catalog.json)，协议结构从Go类型提取；示例位于 [内置示例](web/src/dynamic/example.json)。Go使用embed，不依赖前端打包。更改注册表后重启go run使手册与后端校验同步。

原先重复的注册、组件、CSS、网关和实现契约手册已合并到接口，不再维护并行Markdown版本。部署、安全约束和历史测试记录仍在docs中保留。

## 当前未实现 / 不在第一版范围

- Docker 管理、远程部署、容器生命周期、镜像仓库、Kubernetes、集群调度。
- 通用反向代理、任意 URL 转发、自动公网暴露、统一业务认证。
- 插件市场、任意 JS/HTML 执行与全局CSS注入、多用户/RBAC、OIDC、MFA、在线密码重置。已支持的是受限且自动作用域化的服务页面CSS。
- 任意凭据、Cookie或请求头透传；仅由中心注入该服务的独立Bearer业务令牌。多租户／细粒度终端用户业务授权仍由服务自身实现。
- 自动备份、审计归档/保留期管理、高可用与跨实例同步；单实例 SQLite 是设计目标。

业务鉴权、配置示例与调用边界由接入手册接口提供。更多安全与运行边界见 [docs/security.md](docs/security.md)。
