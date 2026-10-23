package protocol

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	webcontract "workstation/web"
)

// 手册只包含内置协议和示例；不接受配置、数据库记录或凭据作为生成输入。
const integrationGuide = `## 给接入方与AI的工作说明

你正在为个人工作站中心开发独立服务。先阅读本接口全文，再按当前白名单实现；不要猜测组件名、CSS能力或接口格式。
中心和服务独立运行。中心发现新服务的方式是服务主动注册，不扫描网络。中心负责登记、健康检查、启用状态和渲染；不实现你的业务逻辑、不管理Docker、不远程部署。

### 接入顺序

1. 向管理员申请固定服务id、允许登记的origin、注册密钥和本服务独立业务令牌。注册密钥、业务令牌、初始化令牌不能复用，均不得写到页面JSON、前端环境变量、日志或URL。手册不会提供任何真实密钥。
2. 管理员在中心部署配置CONTROLLER_ALLOWED_SERVICE_ORIGINS中加入精确origin，在CONTROLLER_SERVICE_ACCESS_FILE指定的文件中加入本服务origin、独立bearer_token及精确路径/方法白名单，并重启中心。注册请求不能修改这些权限。服务自身读取自己的业务令牌，不要给服务共享所有服务的生产策略文件。
3. 实现健康路径（默认/healthz），快速返回2xx且不含敏感信息。业务API返回application/json，校验独立Bearer令牌、真实TCP来源IP及业务权限；不信任X-Forwarded-For，不向浏览器开放CORS。来源IP白名单不能替代令牌。
4. 服务启动后POST /api/v1/registry/register，Content-Type: application/json，Authorization: Bearer <注册密钥>。使用下方完整注册示例，并替换服务元数据和页面内容。建议每30秒有限频率重试/更新登记；写业务请求不自动重试。
5. 首次登记为pending/unknown。管理员在中心启用后菜单才出现；在线状态和启用状态互相独立，离线不会删除登记。恢复连接无需重新启用。
6. 同一个id重新注册只更新元数据和定义，绝不改变管理员启用、固定和排序选择。错误注册保留旧有效定义。删除只清理中心记录；仍在注册的服务会重新出现并回到pending。要持续隐藏，禁用或停止注册。
7. 用table/form/detail/compose和本文列出的组件定义页面；新增普通服务无需修改中心。新增中心组件时在共享注册表声明字段与描述、实现渲染和语义校验，手册目录自动跟随，不另维护Markdown名单。

### 注册格式和校验

外层protocol_version=1，definition.schema_version=1；menus/pages必须显式为数组，可以同时为空用于只登记。definition可选icon；菜单为{id,label,page_id,icon?}，id唯一，必须指向本服务已存在页面。分组可折叠、拖动排序和固定，但这些是管理员偏好，服务不得在注册JSON里提交pinned/navigation_order。
服务id匹配^[a-z0-9][a-z0-9-]{0,63}$，永久且唯一。name为1–120字，version为1–64字，description可选最多2000字。base_url是单纯HTTP(S)来源，不得有账号密码、路径、query或fragment，且必须在部署白名单内。
页面、菜单、组件、字段与操作id以ASCII字母开头，后跟字母数字_-，最长64字符，禁止constructor/prototype/__proto__；同一相应作用域内唯一。健康与业务path只能用单个/开头，最长512字符，字符限ASCII字母数字和_~./-，禁止URL、//、百分号、反斜线、query、fragment、.或..路径段。
普通文本最多2000字、标签最多120字。所有JSON限制见下方实时注册表；拒绝重复键、未知字段、未知版本和未知组件，不会尝试加载远程模块。CSS字段语义在浏览器挂载时检查，后端只验证形状/大小/编码；非法CSS只影响该页。
面包屑默认“站点 / 服务 / 当前页”，页面breadcrumbs可在服务和当前页之间加入祖先；图标只用下方内置图标名，不能提交SVG/HTML/URL。

### 业务数据契约

浏览器只调用同源中心网关，不直连服务，不保存业务令牌。source.path始终属于本服务origin，不是中心路径。中心只调用已启用服务、已保存页面和部署路径/方法白名单的交集。
table的GET source自动附page（从1开始）和page_size（1–100，默认20）；服务负责分页，返回{"items":[{"id":1,"title":"检查备份"}],"total":1}，total为非负整数。最多返回请求的每页条数，每条记录最多100个字段；值仅string/number/boolean/null。表格操作仅POST或DELETE，发送{"id":行.id}，行必须有可用文本/数字id；不支持路径模板。
form初始source可选，返回平面字段对象；submit仅POST/PUT，发送已声明字段，成功返回JSON对象（例如{"ok":true}），不要返回204/HTML。字段必填、类型、长度、数字范围、真实日期和选项由网关再次校验，业务服务仍需独立校验。空可选字段建议兼容null和空字符串，date未选择时提交null。
detail和compose的source返回最多100字段的平面对象，例如{"task_count":8,"archived_count":2,"completion":25,"owner":"个人管理员"}。key/value_key只读对象自身键，不解析a.b、表达式或模板。缺失/null/空字符串显示占位；所有内容只按纯文本显示。
compose共享source只在挂载或点击刷新时读取；嵌入页使用被引用页面自己的source/submit/action和权限。组合页刷新不自动刷新嵌入页。禁止嵌入compose防止递归；页签离开后卸载、取消请求、丢弃未提交表单。
业务响应最多1MiB，普通文本仍最多2000字、嵌套最多12层；前端字段/元素总量最多20000。上游超时6秒、并发最多8，前端超时8秒；中心接口总超时15秒。响应必须UTF-8 JSON，不支持流式下载、文件上传、WebSocket或任意响应头透传。

### 中心网关调用格式（由中心界面负责）

POST /api/v1/services/{服务id}/pages/{页面id}/invoke；所有source/submit/action均需中心管理员会话和X-CSRF-Token。
读取：{"operation":"source"}；table读取可加"query":{"page":1,"page_size":20}。
表单：{"operation":"submit","body":{"owner":"管理员"}}。
表格操作：{"operation":"action","action_id":"archive","body":{"id":1}}。
浏览器不能提交URL、path、method、自定义请求头或业务凭据。中心不向服务传管理员Cookie/Authorization/Origin/转发头，只注入该服务独立Bearer。上游Cookie/Location及原始错误正文不透传。
部署白名单、会话、CSRF、IP与TLS均是独立边界；不要通过放开认证、任意URL代理或自动暴露公网来解决错误。HTTP携带业务令牌仅限实际回环地址，非回环必须HTTPS并验证证书；中心拒绝重定向、环境代理、危险IP与DNS重绑定。

### 错误与兼容

中心错误格式为{"error":{"code":"机器可读代码","message":"中文说明"}}；按状态码和code处理，不依赖中文文案。注册成功200返回{"service":...}；无注册凭据/错误凭据401，非法输入400，来源/CSRF/策略拒绝403，未找到404，业务定义或响应异常可能422/502/504，中心内部错误500/超时503。真实业务错误在中心显示统一提示，不回显服务的敏感正文。
无效JSON、超限数据、样式错误或渲染异常只影响对应页；离线记录仍能管理、禁用与删除。写请求先记审计意图再记结果；超时不代表未执行，禁止自动重复提交。
协议和schema独立版本化；先升级中心再使用新字段；旧中心拒绝未知能力而非静默降级。此系统仅面向本地可信个人服务，共享注册密钥不隔离服务身份，不能交给不信任的租户。未来跨服务器接入需要单独升级身份及网络策略。

### CSS写法和优先级

页面可配置class_name/style/stylesheet，组件可配置class_name/style。style使用CSS属性名（background-color而非backgroundColor）和字符串值；stylesheet是真实CSS文本，不是模板。推荐用服务序列化器生成JSON以正确转义双引号和换行。根样式用.service-page或页面class_name。
例：.summary { padding: 20px; background: var(--surface-raised); color: var(--text-primary); border: 1px solid var(--border-default); border-radius: 12px; }。
每个选择器自动限定当前页，不能污染控制台和其他服务；不是JS沙箱。支持!important但仅在本页。内联style通常优先，grid的默认列数和gap也通过内联style设置，必要时用局部!important；窄屏内置单列布局请保留。
主题变量自动跟随明暗切换；固定颜色不会自动适配。不要隐藏错误、必填或焦点信息。全局body浮层不接受服务CSS。具体属性/函数/变量/选择器和资源限制由下面的实际CSS注册表生成。

### 独立服务最小职责清单

必须：健康接口、受令牌及IP保护的JSON业务API、启动注册与有限重试、真实业务校验和可持久化业务存储（示例使用内存，重启会清空）。无需：访问中心数据库、写Vue模板、托管组件代码、浏览器CORS、修改中心路由或提交管理员会话。注册密钥与业务令牌由管理员私下安全分发，不能从手册索取。
`

type ContractField struct {
	Name      string `json:"name"`
	GoType    string `json:"go_type"`
	OmitEmpty bool   `json:"omitempty"`
}

// 从实际Go协议结构取字段，避免新字段遗漏在机器可读结构清单中。
func contractFields(value any) []ContractField {
	t := reflect.TypeOf(value)
	out := []ContractField{}
	for index := 0; index < t.NumField(); index++ {
		field := t.Field(index)
		tag := strings.Split(field.Tag.Get("json"), ",")
		if tag[0] == "" || tag[0] == "-" {
			continue
		}
		out = append(out, ContractField{Name: tag[0], GoType: field.Type.String(), OmitEmpty: len(tag) > 1 && tag[1] == "omitempty"})
	}
	return out
}

func RegistrationExample() map[string]any {
	return map[string]any{"protocol_version": 1, "id": "my-service", "name": "我的服务", "version": "1.0.0", "description": "独立服务示例（不是当前部署配置）", "base_url": "http://127.0.0.1:8091", "health_path": "/healthz", "definition": webcontract.ExampleDefinition}
}

func AccessPolicyExample() map[string]any {
	return map[string]any{"my-service": map[string]any{"origin": "http://127.0.0.1:8091", "bearer_token": "<管理员生成的本服务独立随机令牌，至少32字符>", "endpoints": []map[string]any{
		{"path": "/api/overview", "methods": []string{"GET"}},
		{"path": "/api/tasks", "methods": []string{"GET"}},
		{"path": "/api/tasks/archive", "methods": []string{"POST"}},
		{"path": "/api/preferences", "methods": []string{"GET", "PUT"}},
	}}}
}

func HandbookContract() map[string]any {
	return map[string]any{
		"format_version": 1, "protocol_version": 1, "schema_version": 1,
		"generated_from":       []string{"web/src/dynamic/catalog.json（实际组件/样式/图标白名单）", "internal/protocol协议结构", "web/src/dynamic/example.json（内置示例）"},
		"registration":         map[string]any{"method": "POST", "path": "/api/v1/registry/register", "authentication": "Authorization: Bearer <管理员私下提供的注册密钥>", "content_type": "application/json", "fields": contractFields(Registration{})},
		"struct_fields":        map[string]any{"definition": contractFields(Definition{}), "menu": contractFields(Menu{}), "page": contractFields(Page{}), "block": contractFields(Block{}), "field": contractFields(Field{}), "column": contractFields(Column{}), "option": contractFields(Option{}), "action": contractFields(Action{}), "source": contractFields(Source{}), "endpoint": contractFields(Endpoint{}), "breadcrumb": contractFields(Breadcrumb{})},
		"ui":                   webcontract.Catalog,
		"registration_example": RegistrationExample(), "access_policy_example": AccessPolicyExample(),
	}
}

func jsonBlock(value any) string {
	data, _ := json.MarshalIndent(value, "", "  ")
	return "\n```json\n" + string(data) + "\n```\n\n"
}
func orderedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
func describeFields(builder *strings.Builder, fields map[string]string) {
	for _, name := range orderedKeys(fields) {
		fmt.Fprintf(builder, "- `%s`：%s\n", name, fields[name])
	}
	builder.WriteString("\n")
}
func describeEntries(builder *strings.Builder, entries map[string]webcontract.Entry) {
	for _, name := range orderedKeys(entries) {
		fmt.Fprintf(builder, "### %s\n\n%s\n\n", name, entries[name].Summary)
		describeFields(builder, entries[name].Fields)
	}
}

func HandbookMarkdown() string {
	var builder strings.Builder
	builder.WriteString("# 中心服务接入手册\n\n本手册由实际协议结构、组件/CSS注册表与内置示例自动生成；不包含当前部署配置、服务记录或密钥。\n\n协议版本1 / 页面schema版本1。同一路径加 `?format=json` 获取结构化契约、完整说明与示例。接口由管理员在系统设置开关，关闭时返回404，无论是否登录均不返回手册。\n\n")
	builder.WriteString(integrationGuide)
	builder.WriteString("\n\n## 注册与部署白名单完整示例\n\n以下地址与占位令牌只是示例，不是可直接使用的部署凭据。注册请求正文：\n")
	builder.WriteString(jsonBlock(RegistrationExample()))
	builder.WriteString("中心侧部署策略文件示例（不是注册请求的一部分；另需允许该origin并重启中心）：\n")
	builder.WriteString(jsonBlock(AccessPolicyExample()))
	builder.WriteString("## 页面通用字段\n\n")
	describeFields(&builder, webcontract.Catalog.PageCommon)
	builder.WriteString("## 页面类型（由校验器注册表生成）\n\n")
	describeEntries(&builder, webcontract.Catalog.PageTypes)
	builder.WriteString("## 组件通用字段\n\n")
	describeFields(&builder, webcontract.Catalog.BlockCommon)
	builder.WriteString("## 组合组件（由校验器注册表生成）\n\n")
	describeEntries(&builder, webcontract.Catalog.Components)
	builder.WriteString("## 表单字段\n\n")
	describeFields(&builder, webcontract.Catalog.FormFields)
	builder.WriteString("## 表单控件类型\n\n")
	describeFields(&builder, webcontract.Catalog.FormTypes)
	builder.WriteString("## CSS实际白名单与作用域规则\n\n数组为实际支持值，variables内名称使用时加--前缀。\n")
	builder.WriteString(jsonBlock(webcontract.Catalog.CSS))
	builder.WriteString("## 实际资源限制\n\n")
	builder.WriteString(jsonBlock(webcontract.Catalog.Limits))
	builder.WriteString("## 内置图标\n\n")
	builder.WriteString(strings.Join(orderedKeys(webcontract.Catalog.Icons), "、"))
	builder.WriteString("\n\n## 实际Go协议字段\n\nGo的omitempty只表示序列化省略空值，不代表业务上可选；必填条件以本文字段说明和协议校验为准。按type使用专用字段，不混用。\n")
	builder.WriteString(jsonBlock(HandbookContract()["struct_fields"]))
	return builder.String()
}
