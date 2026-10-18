# FlClash 仓库代理指南

本文件是整个仓库的代理工作入口。详细规则保留在 `.agents/`，专项操作按任务加载对应技能。

## 开始工作

1. 阅读 [PROJECT.md](PROJECT.md)、[项目背景](.agents/project.md)、[开发命令](.agents/commands.md) 和 [编码规则](.agents/rules.md)。
2. 检查 `git status --short`，识别已有改动；只修改任务需要的内容，不覆盖、清理或回退他人的工作。
3. 从实际调用入口定位问题，先读相关实现和测试，再修改。范围明确、风险低的任务直接实施，不额外要求设计文档或反复确认。
4. 涉及跨平台行为、协议、数据库或生命周期时，先阅读 [架构说明](.agents/architecture.md)，明确状态与资源由谁管理。
5. 完成后检查差异、运行与改动对应的验证；区分已通过、失败、跳过和未验证，不以静态检查代替设备验证。

## 项目与技术栈

- FlClash 是基于 ClashMeta（mihomo）的 Flutter 代理客户端，支持 Android、Windows、macOS、Linux；没有 iOS 或 Web 构建入口。
- 当前仓库是 `ani13-teach/FlClash` 分支，上游为 `chen08209/FlClash`。README 中的上游下载入口不代表本分支产物。
- 界面使用 Flutter 和 Material You；状态管理使用 Riverpod；模型和序列化使用 Freezed、JSON 生成器；数据库使用 Drift/SQLite。
- Go 提供 Core 封装，Rust 提供 QuickJS、桌面 IPC 和 Helper。Android 也需要 Rust，不是纯 Flutter 工程。
- 应用版本与依赖约束查 `pubspec.yaml`，解析版本查 `pubspec.lock`；Flutter/Go 构建基准查 `.github/workflows/build.yaml`，Rust 查 `plugins/rust_api/rust/rust-toolchain.toml`，Android SDK/NDK 查 `android/gradle/libs.versions.toml`。
- 不凭历史说明降级依赖，也不把 CI 固定版本当成最低兼容版本。替换 Git fork 依赖前核对分支专有行为和调用方。

## 目录与职责

| 路径 | 职责 |
| --- | --- |
| `lib/main.dart`、`lib/bootstrap.dart`、`lib/application.dart` | 应用入口、初始化和管理器组合 |
| `lib/views/`、`lib/pages/`、`lib/widgets/`、`lib/features/` | 页面、组件和功能界面 |
| `lib/providers/` | Riverpod 状态与业务操作，包括 `actions/`、`state/` |
| `lib/models/`、`lib/database/` | 模型、序列化、数据库和迁移 |
| `lib/common/`、`lib/manager/` | 通用能力与平台管理 |
| `lib/core/`、`lib/core/desktop/` | Flutter Core 接口、平台桥接、桌面 IPC 与进程生命周期 |
| `core/`、`core/Clash.Meta/` | Go 封装与 mihomo Git 子模块 |
| `android/`、`windows/`、`macos/`、`linux/` | 平台宿主工程 |
| `plugins/` | 本地插件：`proxy`、`rust_api`、`setup`、`tray`、`wifi_ssid` |
| `services/helper/` | Windows/Linux 特权 Helper |
| `arb/`、`lib/l10n/` | 翻译源文件与生成输出 |
| `assets_source/`、`assets/` | 图标源文件与运行资源 |
| `test/`、`android/tests/` | 根包测试与 Android JVM 测试 |
| `tool/`、`setup.dart` | 维护脚本与完整打包入口 |

`build/`、`dist/`、`libclash/`、`.dart_tool/` 是产物或缓存。不要把 `core/Clash.Meta/` 当作缓存删除。`docs/` 被忽略，可维护的仓库说明放在现有文档体系内。

## 不可破坏的边界

- **桌面 Core**：进程由 `DesktopCoreLifecycle` 管理，通过 `CoreProcessLease` 获取和释放；页面、provider、manager 不自行启动或杀死 Core。
- **Android 服务**：最新启动/停止意图由原生 `ServiceState` 仲裁，`ServiceController` 管理绑定和运行。Flutter 命令确认不等于 VPN 已建立，不在 Flutter 增加第二套生命周期控制。
- **退出与异步**：关闭保持终结性和幂等性；应用退出集中在 `SystemAction`/`SystemExitCoordinator`。重叠的启动、停止、重启必须服从最新意图，旧请求不得覆盖新状态。
- **跨语言协议**：保持 Dart、Go、JNI 和桌面 IPC 的请求、响应及事件结构一致，不二次编码参数、结果或事件批次。修改时更新对应契约测试。
- **安全边界**：不通过 Core 或 Helper IPC 暴露任意文件删除；保留连接身份校验。优先在 FlClash 封装侧修复问题，修改 mihomo 子模块须符合 `.agents/rules.md` 的范围限制。
- **Sticky 故障转移**：由修改版 Core 执行，Flutter 只显示选择；当前节点健康时不主动切换，故障转移后不因原节点恢复自动切回。精确前缀为 `[Sticky] `，配套脚本为 `tool/sticky_override.js`。

## 编码与生成规则

- 遵循根目录 `lint_options.yaml`：单引号、多行尾逗号、`child:` 最后、明确返回类型，优先 `const`/`final`，禁止 `print()` 和抛出裸字符串。
- 手写 Material 界面使用 `material_ui`，不新增 `package:flutter/material.dart` 或 `cupertino_ui` 导入；生成的本地化文件保留生成器输出。
- 圆角遵循仓库的 superellipse API。图标按钮提供可访问名称；自建 controller、notifier、stream 等资源由所有者释放。
- 注释仅保留代码不能直接表达的约束，不逐行解释、不叙述改动历史、不保留注释掉的代码。新增行的独立注释密度受 5% 门槛约束；保留许可证、生成标记和 lint 指令。
- 不手改 `lib/models/generated/`、`lib/providers/generated/`、`lib/database/generated/`、`lib/l10n/l10n.dart` 和 `lib/l10n/intl/`。
- 修改模型、provider 或数据库结构后重新生成，并检查迁移和相关测试：

  ```bash
  dart run build_runner build --delete-conflicting-outputs
  ```

- 新增界面文案同时修改 `arb/intl_en.arb`、`arb/intl_zh_CN.arb`、`arb/intl_ja.arb`、`arb/intl_ru.arb`，不硬编码文案；然后生成：

  ```bash
  dart run intl_utils:generate
  ```

## 开发与验证

以下命令从仓库根目录执行。首次准备环境时初始化子模块、安装依赖，再选择设备运行：

```bash
git submodule update --init --recursive
flutter doctor -v
flutter pub get
flutter devices
flutter run -d <设备ID> --dart-define=APP_ENV=dev
```

常规代码改动使用以下检查，并先运行受影响的测试以便定位失败：

```bash
dart format --output=none --set-exit-if-changed lib test tool plugins setup.dart
flutter analyze --no-fatal-infos
flutter test --reporter expanded
```

- 根包必须用 `flutter test`，不是 `dart test`。根包分析与测试不能覆盖全部本地插件，插件检查使用 `bash tool/check_plugins.sh`。
- `plugins/setup/setup_hooks/` 是独立纯 Dart 包，进入该目录运行 `dart test`，不将它混入根包 Flutter 测试。
- 根包测试和 `dart run` 也可能触发 Go/Rust 原生构建。查看 `.dart_tool/setup_build_cache/hook.log` 定位构建问题；不通过提交关闭原生 hook 来掩盖失败。
- `pubspec.yaml` 的 `setup`、`rust_api` 两项 `build_assets` 在交付与打包时必须为 `true`。如为本地纯 Dart 测试临时关闭，完成后恢复。
- provider 测试优先注入 scoped Core 假实现，避免全局单例和真实系统副作用；自动释放的 provider 在测试中保持监听。具体做法见对应技能。
- CI 检查总体覆盖率 75% 和目录分组门槛，不降低门槛来通过验证；不为 `core/` 新增覆盖率采集。

按改动范围追加验证：

| 范围 | 命令或操作 |
| --- | --- |
| Go Core | 在 `core/` 执行 `CGO_ENABLED=0 go test .`、`CGO_ENABLED=0 go vet .` |
| Sticky 脚本 | `node --test tool/sticky_override_test.cjs` |
| Rust API | `cargo test --manifest-path plugins/rust_api/rust/Cargo.toml` |
| Helper | `cargo test --manifest-path services/helper/Cargo.toml`；Windows 服务专项按命令文档执行 |
| Android 服务 | JDK 17 下编译相关模块、运行对应 JVM 测试；Windows 使用 `gradlew.bat` |
| VPN/TUN、系统授权、锁屏和服务回收 | 对应平台真机或模拟器验证，不能仅凭编译和单元测试判定通过 |

## 打包与发布

- 相比上次打包，只要源码变更，打包前必须增加 `pubspec.yaml` 的构建号，本地测试包同样适用。
- 默认运行 `bash tool/bump_version.sh minor`，只递增 `+` 后的构建号，并核实大于上次打包值；不额外增加主版本、次版本或修订号。源码未变的重复构建无需升级。
- 用户要求打包或发布安装包时，默认交付正式版安装包，使用 `release` 构建模式与 `stable` 应用环境；只有用户明确要求时才交付 debug 或其他开发、预发布安装包。
- 完整打包使用 `dart setup.dart <平台> --env stable`，例如 `dart setup.dart android --arch arm64 --env stable`。桌面只能在对应宿主系统构建；输出在 `dist/`，完整参数见 `.agents/commands.md`。
- `setup.dart` 不指定环境时默认为 `pre`，因此正式版打包必须显式传入 `--env stable`。该脚本会修改本地 `env.json`、全局激活打包工具，部分平台还可能安装系统依赖；不要把它当作只读验证命令。
- Android 的 `--env stable` 不配置正式签名。正式版交付前必须核实正式签名与 `com.follow.clash` 包名；缺少完整签名配置时，现有构建会回退到 debug 签名与 `com.follow.clash.dev` 包名，此时停止正式版交付并说明缺少的签名配置，不默认以回退产物替代正式版。不为覆盖安装擅自卸载应用或更改签名。
- 未明确要求时，不提交、推送、创建发布标签或执行发布操作。推送 `v*` 标签会触发发布链路。
- 本文件 `AGENTS.md` 纳入 Git 版本管理。用户要求将本仓库提交或推送到远端时，检查本文件的变更并与本次改动一并提交、推送，无需单独提醒；没有变更时无需重复提交或上传。若本文件已有本地提交但尚未推送，应随本次推送同步。不为同步指南额外创建发布标签或触发安装包发布。
- 需要提交时遵循 Conventional Commits；禁止添加署名给编码代理的 `Co-authored-by`。
- 不提交订阅 token、节点密码、WebDAV 凭据、签名密钥和个人配置；日志与复现材料脱敏，不删除用户数据来解决构建或安装问题。

## 专项指引

| 任务 | 必读技能 |
| --- | --- |
| UI、组件、交互和异步反馈 | [ui-work](.agents/skills/ui-work/SKILL.md) |
| 文案、ARB 和本地化生成 | [localization](.agents/skills/localization/SKILL.md) |
| Riverpod 状态和 provider 测试 | [provider-tests](.agents/skills/provider-tests/SKILL.md) |
| Core、服务、IPC、VPN/TUN、Helper | [core-platform](.agents/skills/core-platform/SKILL.md) |
| Sticky 分组、探测、持久化、选择展示及专项 APK | [sticky-failover](.agents/skills/sticky-failover/SKILL.md) |

修改代理配置前阅读 [.agents/agent-config.md](.agents/agent-config.md)；使用额外工作树前阅读 [.agents/worktrees.md](.agents/worktrees.md)。工具链、协议、数据库或生命周期变更应同步维护对应文档，不另建与现有规则相冲突的说明。
