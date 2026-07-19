# CLI:qorm

> `qorm` 二进制的手工编写参考(实现于 `cmd/qorm/`)。与本站其他页面不同,本页不是生成的——CLI 变化时请同步更新本页。

一个二进制即是完整工具链:脚手架、运行、渲染、度量、验证、签名、打包、发布,以及面向智能体的服务。默认纯 Go 构建(可交叉编译到各平台);`-tags desktop` 构建额外提供原生 WebView,`shot` / `measure` / `check` / `preview` 与 `run --app` 都依赖它。

| 命令 | 作用 |
|---|---|
| [`new`](#new) | 生成可运行的应用脚手架 |
| [`run`](#run) | 实时运行应用(浏览器与智能体共享同一运行时) |
| [`render`](#render) | 输出静态 HTML 快照 |
| [`shot`](#shot) | 把应用 / 页面 / 窗口栅格化为 PNG(macOS,`-tags desktop`) |
| [`measure`](#measure) | 渲染并自我度量布局与样式(`-tags desktop`) |
| [`check`](#check) | 对照期望验证渲染结果(`-tags desktop`) |
| [`build`](#build) | 编译(并可选签名)捆绑包 |
| [`keygen`](#keygen) | 生成 ed25519 签名密钥对 |
| [`sign`](#sign) | 为已有捆绑包签名 |
| [`verify`](#verify) | 验证捆绑包的完整性 / 签名 / 吊销状态 |
| [`mcp`](#mcp) | 通过 MCP(stdio)把应用提供给智能体 |
| [`preview`](#preview) | 渲染已打包的应用并报告其布局(`-tags desktop`) |
| [`package`](#package) | 打包为可安装应用(web / iOS / Android / mac / 小程序) |
| [`docs`](#docs) | 把 markdown 文档树渲染为静态 HTML 站点 |
| [`audit`](#audit) | 验证哈希链式活动审计日志 |
| [`updates`](#updates) | OTA 发布服务器,支持分阶段(金丝雀)发布 |
| [`update`](#update) | 自更新 CLI(验证签名校验和) |
| [`version`](#version) | 打印版本 |

## 约定

- **输入。** `<app-dir>` 是应用源码目录(`qorm.json` + `scenes/` + `actions/`)。`run`、`render`、`mcp` 也接受编译后的 `.qorm.bundle`——使用前会先验证(始终验证完整性;给出 `--trust` 时验证真实性;给出 `--revoked` 时验证吊销)。`render` 还接受单个场景 JSON 文件。
- **退出码。** `0` 成功,`1` 运行时或验证失败,`2` 用法错误(未知命令也是 `2`)。
- **标志。** `-o` 与 `--out` 处处等价,`-p` 与 `--platform` 同理。标志为手工解析:大多数命令会把无法识别的记号当作位置参数;`qorm update` 例外,会拒绝未知标志。
- **环境变量。**

| 变量 | 使用者 |
|---|---|
| `QORM_KEYSTORE_PASS` / `QORM_KEY_PASS` | `package -p android --release`——keystore / key 密码(否则交互询问) |
| `ANDROID_HOME` / `ANDROID_SDK_ROOT` | `package -p android`——定位 Android SDK |

- **无参数。** 在打包好的 macOS `.app` 内,裸 `qorm` 会运行内嵌的应用;其他场合打印用法并以 `2` 退出。
- **内部命令。** `__release-sign`、`__logwin`、`__tray` 是内部子进程辅助命令(CI 发布签名、桌面日志窗口、系统托盘)——不是用户命令;`__release-sign` 从 `QORM_RELEASE_KEY` 读取发布密钥。

## new

```
qorm new <dir> [--name "App Name"]
```

生成最小可运行应用的脚手架:`qorm.json`(清单;应用 `id` 由目录名净化而来)、`scenes/main.json`(一个计数器界面)、`actions/inc.json`。拒绝非空目录(退出码 `1`)。`--name` 设置显示名(默认:目录基名)。

## run

```
qorm run <app-dir|bundle> [标志]
```

实时提供应用服务:浏览器 UI、SSE 推送通道与 `/mcp` 智能体端点共享同一运行时。端点契约见 [HTTP 与 SSE](http-api.md)。

| 标志 | 效果 |
|---|---|
| `--port N` | 监听端口(默认 `10383`;被占用时回退到随机空闲端口并打印) |
| `--no-open` | 不打开浏览器窗口 |
| `--app` | 独立窗口:`-tags desktop` 构建下为原生 WebView;否则为无铬边的 Chromium 系窗口(`--app=<url>`),再退化为普通标签页 |
| `--console` | 打开协作控制台(`/console`)而不是应用页 |
| `--lan` | 绑定 `0.0.0.0`,让物理设备加入同一会话;打印 Wi-Fi URL,并为已连接的 Android 设备执行 `adb reverse`;隐含 `--no-open` |
| `--tls` | 使用覆盖 localhost 与各 LAN IP 的自签名证书提供 HTTPS(安全上下文,设备上相机/麦克风/定位等 Web API 所必需);隐含 `--lan` |
| `--mcp-read-only` | 共享 MCP 会话拒绝产生变更的工具(`qorm_dispatch`、`qorm_set_state`、`qorm_apply_patch`、`qorm_undo`);检查与预览类工具照常工作 |
| `--no-watch` | 关闭热重载 |
| `--trust pub.key` | 要求捆绑包携带该密钥的有效签名 |
| `--revoked list.json` | 拒绝由已吊销密钥签名的捆绑包 |
| `--audit-log file` | 把每条活动条目追加到哈希链式 JSONL 日志(用 [`qorm audit`](#audit) 验证);链在多次重启间延续 |

行为说明:

- **目录**输入会热重载:400 毫秒 mtime 轮询,文件变化时重新解析应用并把重渲染推送给每个客户端,保留状态/场景/视口。写到一半的文件会被报告,当前应用保持不变。
- **捆绑包**输入在提供服务前先验证,并启用 OTA 端点(`/update`、`/rollback`)。不带 `--trust` 时只做完整性校验——stderr 会警告真实性未验证。
- 加载器的静态诊断(废弃属性、表达式类型不匹配)打印到 stderr,从不阻止运行。

## render

```
qorm render <app-dir|scene.json|bundle> [-o out.html]
```

输出静态 HTML 快照(服务端首帧;无实时客户端)。默认输出:`<输入基名>.html`。捆绑包输入只做完整性校验——本命令没有 `--trust`,因此会警告真实性未验证。

## shot

```
qorm shot <app-dir> -o out.png [--width W --height H]
qorm shot --html page.html -o out.png
qorm shot --url URL -o out.png
qorm shot --live "窗口标题" -o out.png
```

经离屏 WebKit WebView 栅格化为 PNG。**仅 macOS + `-tags desktop`**——其他构建打印错误并以 `2` 退出。需要 GUI 会话,且终端需有"屏幕录制"权限(截图经由系统 `screencapture` 工具)。捕获前 CSS 动画被冻结在最终状态。

- 应用目录:先渲染再捕获(默认 `--width 440 --height 720`,默认输出 `<目录>.png`)。
- `--html`:捕获静态 HTML 文件(默认输出 `<名称>.png`)。
- `--url`:捕获实时 URL——适用于 `/console` 这类依赖服务端的页面(默认输出 `shot.png`)。
- `--live`:按标题捕获一个已在运行的窗口(先精确匹配标题,再子串匹配),不启动新的 WebView(默认输出 `shot.png`)。

## measure

```
qorm measure <app-dir> [--width N] [-o report.json]
```

在原生 WebView 中渲染应用,让它自我度量,然后每个节点输出一行 JSON,把**意图**(类型 / 文本 / 绑定)与**渲染结果**(矩形 + 计算样式)对应起来——打到 stdout,或写入 `-o`。视口宽度默认 400(高度固定 820)。**需要 `-tags desktop` 构建**;其他构建以 `1` 退出。报告字段见[解读与验证 QORM 应用](/docs/zh/verification.html)。

## check

```
qorm check <app-dir> (--checks checks.json | --audit) [--width N] [-o report.json]
```

像 [`measure`](#measure) 一样度量应用,再对照真实渲染评估期望。**需要 `-tags desktop` 构建。**

- `--checks checks.json`——可以是 `{ "id": …, <断言>… }` 静态检查数组(可见性、文本、几何、对比度、ARIA 等),也可以是 `{"steps":[…]}` 流程对象:每一步先执行 `do.dispatch "<动作>"` 或 `do.setState`,等待重渲染与重度量,再执行该步的检查——即交互测试。
- `--audit`——无需手写检查:对每个**可见**组件验证通用不变量(尺寸非零、无水平溢出、在窗口内)。

**退出状态:** 即使检查失败也是 `0`——通过与否在报告的 `ok` 字段中。只有运行时错误(检查 JSON 非法、加载失败)才非零。断言模式与报告格式见[验证](/docs/zh/verification.html)。

## build

```
qorm build <app-dir> [-o app.qorm.bundle] [--key priv.key] [--version V] [--require-capability camera,location]
```

把应用编译为单个捆绑包(`qorm-bundle/1` JSON):内容(清单 + 场景 + 动作 + 多语言)、对规范编码计算的 `contentHash`,以及——给出 `--key` 时——分离式 ed25519 签名。默认输出:`<目录基名>.qorm.bundle`。

- `--version V`——在签名前把版本盖入清单(被哈希覆盖)。
- `--require-capability a,b`——把所需能力记入被签名的内容;运行时在缺少任一能力的平台上拒绝启动该捆绑包。
- 静态诊断打印到 stderr;不导致构建失败。

## keygen

```
qorm keygen [--out-dir .]
```

生成 ed25519 密钥对:`--out-dir` 下的 `qorm_key`(私钥,权限 `0600`)与 `qorm_key.pub`,并打印 12 字符密钥 id。密钥文件为两行文本——一行头部加 base64 密钥字节——便于查看与搬运。

## sign

```
qorm sign <bundle> --key priv.key [-o out]
```

为已有捆绑包签名——例如经 MCP `qorm_export_bundle` 工具从实时设计会话导出的捆绑包。**不带 `-o` 时原地覆盖输入文件。**

## verify

```
qorm verify <bundle> [--trust pub.key] [--revoked list.json]
```

分层验证捆绑包并打印已证明的范围:始终验证完整性(重算内容哈希),`--trust` 时 `+ signature`,`--revoked` 时 `+ revocation`。同时打印捆绑包声明的所需能力。

- 吊销列表是 JSON 数组 `["keyid", …]` 或对象 `{"revoked": […]}`;校验针对实际验证用的密钥,而非捆绑包自声明的 `keyId`。
- 失败时在 stderr 打印 `VERIFY FAILED: <原因>` 并以 `1` 退出。

## mcp

```
qorm mcp <app-dir|bundle> [--trust pub.key] [--revoked list.json]
```

通过 MCP(stdio JSON-RPC)把应用提供给 AI 智能体——与运行中的 [`qorm run`](#run) 在 `/mcp` 暴露的是同一套工具,但使用独立的私有运行时(无共享浏览器会话)。工具参考见 [MCP 工具](/docs/zh/agent/mcp-tools.html)。

## preview

```
qorm preview <package-dir> [--width N] [--eval JS] [-o report.json]
```

验证的是**已打包**的应用而非源码:伺服 `qorm package -p web` 的静态输出,让其客户端 WASM 运行时在无应用服务器的情况下启动并渲染,并捕获应用的自我度量(stdout,或 `-o`)。`--eval JS` 在首次度量后于页面内执行 JavaScript——例如 `qorm(0)` 按下第一个动作按钮——然后重新度量,从而检验打包产物的交互性。**需要 `-tags desktop` 构建。**

不要与 MCP `qorm_preview_patch` 工具混淆,后者是在实时会话上预览设计补丁。

## package

```
qorm package <app-dir> [-p web|ios|android|mac|miniapp] [-o out-dir] [标志]
```

把应用编译为可安装、完全离线的包。默认平台 `web`;默认输出 `<应用目录名>-<平台>`。构建前会向 stderr 打印能力矩阵,就目标平台不支持的功能(以及目标平台缺失的原生中间层)给出警告。

| 平台 | 产物 |
|---|---|
| `web` | 可安装、可离线的 PWA:`index.html` + `bundle.json` + `qorm.wasm` + `wasm_exec.js` + manifest + 图标 + `sw.js` |
| `ios` | Xcode 工程;装有 `xcodegen` + `xcodebuild` 时还会直接构建(未签名的模拟器构建;给出 `--team` 时为已签名真机构建) |
| `android` | Gradle 工程;装有 `gradle` + Android SDK(`ANDROID_HOME` / `ANDROID_SDK_ROOT`)时还会直接构建(wrapper 固定 Gradle 8.9) |
| `mac` | 内嵌桌面二进制的 macOS `.app`(需要 macOS + cgo);开发期使用 ad-hoc 代码签名 |
| `miniapp` | 微信风格小程序工程(WXML/WXSS 静态导出初始 UI;在微信开发者工具中打开) |

`web`/`ios`/`android` 的产物会用 `go build`(`GOOS=js GOARCH=wasm`)编译客户端运行时,因此必须能访问 Go 工具链与 QORM 模块——请在 QORM 仓库内,或 `go.mod` 依赖 `github.com/qorm/qorm` 的目录中运行。应用自己的 Go 中间层(`native/desktop.go`)会被注入该构建,`mac` 平台则注入桌面二进制。

通用标志:

| 标志 | 效果 |
|---|---|
| `--dev URL` | (仅 ios/android)构建连接实时 `qorm run --lan` 服务器的轻薄 **QORM Dev** 客户端——装一次,所有应用复用,改动热重载。与 `--release` 互斥 |
| `--team ID` | iOS 签名的 Apple 开发团队 |
| `--no-branding` | 去掉 "Made with QORM" 标注 |
| `--subscribed` | 非交互地确认 QORM 会员资格(见下) |
| `--update-url URL` + `--trust pub.key` | 把包接入 OTA 更新服务器。两个标志**必须成对给出**(失败即关闭:更新仅在被信任密钥签名时才应用);URL 必须是 http(s) |

**商业闸门(荣誉制度)。** 应用目录中的自定义 `icon.png`,或 `--no-branding`,属于商业化白标:打包器会打印说明并要求确认 QORM Patreon 会员——交互确认,或经 `--subscribed`。非交互运行且不带 `--subscribed` 会失败(退出码 `1`)。个人 / 教学 / 开源使用(默认图标、保留品牌标注)永不触发。

发布标志——`qorm package … --release` 产出可分发、已签名的产物:

| 标志 | 平台 | 效果 |
|---|---|---|
| `--app-version V` | 全部 | 市场版本号(默认 `1.0`) |
| `--build N` | 全部 | 构建号(默认 `1`) |
| `--export-method M` | ios | 导出方式(默认 `app-store-connect`) |
| `--upload` | ios | 导出后上传 App Store Connect(TestFlight) |
| `--api-key F --api-key-id ID --api-issuer UUID` | ios | 无人值守上传用的 App Store Connect API 凭据 |
| `--keystore F --key-alias A` | android | 用已有 keystore 签名(默认别名 `qorm`;密码取自 `QORM_KEYSTORE_PASS` / `QORM_KEY_PASS` 或交互询问) |
| `--apk` | android | 在 AAB 之外再产出已签名 APK |
| `--identity "Developer ID Application: …"` | mac | 签名身份(缺省自动发现) |
| `--notarize [--keychain-profile P]` | mac | 用 `notarytool` 公证 |
| `--no-dmg` | mac | 跳过 DMG 镜像 |

Android 发布签名不带 `--keystore` 时使用托管 keystore `<app-dir>/.qorm/release.keystore`:首次发布用 `keytool` 生成(需要 JDK),密码存于 `keystore.properties`(权限 `0600`),整个 `.qorm` 目录已被 git 忽略,并反复复用,使每次更新签名一致。macOS `--release` 构建绝不回退到 ad-hoc 签名——直接失败。

## docs

```
qorm docs [--docs docs] [-o site] [--name Name]
```

把 markdown 文档树渲染为静态 HTML 站点(本文档站即由它构建)。默认值:源 `docs/`,输出 `site/`,页眉标签 = 源目录基名。页眉会盖上执行渲染的 `qorm` 二进制的版本。

## audit

```
qorm audit <audit-log.jsonl>
```

验证 `qorm run --audit-log` 写出的哈希链式活动日志:每条目的哈希覆盖前一条目的哈希加上自身的序号、时间戳、来源与详情,因此任何被编辑、删除、重排或改换来源的条目都会破坏链条。成功打印 `AUDIT OK: N entries, hash chain intact`;失败打印 `AUDIT FAIL after N verified entries: <原因>`(定位第一个坏条目)并以 `1` 退出。链从首条自锚定——在带外保存最终哈希的副本,才能同时检出截断。

## updates

```
qorm updates [bundles-dir] [--port N]
```

OTA 的发布侧:一个 HTTP 服务器,按分阶段(金丝雀)发布把客户端应运行的捆绑包发给它。默认值:目录 `.`,端口 `0`(随机空闲端口,启动时打印)。

- `bundles-dir/rollout.json` 把应用 id 映射为 `{"stable": "app-v1.qorm.bundle", "canary": "app-v2.qorm.bundle", "canaryPercent": 10}`(`canary` / `canaryPercent` 可选)。文件在启动时读取一次。
- `GET /resolve?app=<id>&client=<id>` 返回该客户端应运行的捆绑包 JSON:当 `FNV-1a(client id) % 100 < canaryPercent` 时给金丝雀版——按客户端确定——否则给稳定版。未知应用 id 与缺失文件返回 `404`;不是合法捆绑包的文件永不发放。
- `GET /bundles/<file>` 直接伺服捆绑包文件。所有路由开放 CORS(`*`)——打包外壳是跨源调用的;信任来自客户端侧的 ed25519 验证(见 [`qorm package --update-url`](#package)),而非谁能读取捆绑包。

## update

```
qorm update [--insecure-skip-verify]
```

从最新 GitHub release 自更新 CLI。已是最新(release 标签等于运行版本):打印说明并以 `0` 退出。否则,有 Go 工具链时执行 `go install github.com/qorm/qorm/cmd/qorm@latest`;没有则下载 `qorm-<os>-<arch>[.exe]` 资产,用构建内嵌的发布公钥对照 release 的 `SHA256SUMS` + `SHA256SUMS.sig` 验证(没有内嵌密钥的构建无法验证,会拒绝),然后替换可执行文件——旧二进制先改名为 `<exe>.old`,替换失败时恢复。`--insecure-skip-verify` 跳过验证直接安装(打印警告;不推荐)。

## version

```
qorm version        # 别名:--version、-v
```

打印 `qorm <版本> (<go 版本> <os>/<arch>)`。版本在构建时经 `-ldflags -X main.version=<tag>` 盖入;未盖章的构建报告源码内的开发默认值。
