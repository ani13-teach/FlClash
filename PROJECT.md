# 项目说明与开发指南

本指南面向使用者和首次接手的开发者。项目简介见 [中文 README](README_zh_CN.md)；详细维护约束见 [AGENTS.md](AGENTS.md)。

## 1. 项目是什么

FlClash 是基于 ClashMeta（mihomo）的代理客户端，使用 Flutter 构建界面，支持 Android、Windows、macOS、Linux。当前没有 iOS 或 Web 应用构建入口。

主要功能包括订阅和配置导入、代理组与规则管理、延迟测试、连接及日志查看、系统代理或 VPN/TUN、主题设置、WebDAV 备份同步。应用本身不提供代理节点，需要用户准备兼容的配置或订阅。

本仓库是 `ani13-teach/FlClash`，上游是 `chen08209/FlClash`。README 中的下载徽章、F-Droid、Homebrew 和 Telegram 链接指向上游，不代表本分支构建产物。当前应用版本以 [pubspec.yaml](pubspec.yaml) 的 `version` 为准，上游历史说明见 [CHANGELOG.md](CHANGELOG.md)。

### 本分支的 Sticky 故障转移

本分支包含 `[Sticky] ` 代理组支持，配套脚本是 [tool/sticky_override.js](tool/sticky_override.js)，完整使用方法见 [Sticky 说明](tool/sticky-failover.md)。

- 把脚本用于订阅的脚本覆写并应用配置，必须搭配本分支的修改版 Core；仅把脚本导入官方客户端不会获得自动故障转移。
- 当前节点健康时保持选择，故障时切到首先确认可用的备用节点，原节点恢复后不会主动切回。
- 健康检测只验证指定 HTTP 地址，不保证所有网站或 UDP 都能使用；节点名称过滤也不是 IP 地理位置认证。
- 自动切换作用于新连接，不保证 Android 锁屏时严格每秒探测，也不保证进程不会被系统回收。
- Flutter 只显示 Core 的选择，不承担第二套故障转移逻辑。

## 2. 普通用户从哪里开始

1. 安装与你的系统和 CPU 架构匹配的应用；区分上游发布包与本分支自行构建的包。
2. 在配置/订阅界面导入订阅链接或配置文件，选中需要使用的配置，检查代理组选择。
3. 按需启用系统代理或 VPN/TUN。Android 首次启用 VPN 需要系统授权；桌面 TUN 的授权与 Helper 行为见下文。
4. 无法联网时先看应用日志、当前配置和节点延迟，不要直接删除数据或卸载应用。
5. 升级或修改配置前，使用备份功能保存数据；WebDAV 同步需要你自己的服务器和凭据。

Linux 使用时需要托盘依赖：

```bash
sudo apt-get install libayatana-appindicator3-dev
```

Android 提供 `com.follow.clash.action.START`、`STOP`、`TOGGLE` 三个动作，具体完整字符串见 README。这些动作不是跨平台命令行接口。

## 3. 源码目录和职责

| 路径 | 用途 |
| --- | --- |
| `lib/main.dart`、`lib/bootstrap.dart`、`lib/application.dart` | 入口、启动初始化、应用和管理器组合 |
| `lib/views/`、`lib/pages/`、`lib/widgets/`、`lib/features/` | 页面、通用组件和功能界面 |
| `lib/providers/` | Riverpod 状态、派生状态和业务操作；实现分布在 `actions/`、`state/` 等子目录 |
| `lib/models/`、`lib/database/` | 数据模型、Drift/SQLite、迁移和 DAO |
| `lib/common/`、`lib/manager/` | 通用工具、平台集成和运行状态管理 |
| `lib/core/` | Flutter Core 接口、Android 桥接、桌面 IPC 和生命周期 |
| `core/` | FlClash Go 封装、协议、事件交付和 Sticky 引擎 |
| `core/Clash.Meta/` | mihomo Git 子模块，不能当作普通生成目录删除 |
| `android/`、`windows/`、`linux/`、`macos/` | 各平台宿主工程 |
| `plugins/` | 本地插件：系统代理、Rust API、构建 hook、托盘、Wi-Fi SSID |
| `services/helper/` | Windows/Linux 特权 Helper，用于需要权限的 Core/TUN 启动 |
| `arb/`、`assets/`、`assets_source/` | 翻译、运行资源、图标源文件 |
| `test/`、`android/tests/` | Flutter/Dart 测试、Android JVM 测试；Go/Rust/插件测试另在各自包内 |
| `tool/`、`setup.dart`、`.github/workflows/build.yaml` | 维护脚本、完整打包、CI |
| `.agents/` | 项目约束、架构、命令和专用维护技能 |

`build/`、`dist/`、`libclash/`、`.dart_tool/` 是本地构建输出或缓存，不是源码。`docs/` 当前被 `.gitignore` 忽略，因此可维护的项目说明放在本文件和 `.agents/`，不要把唯一说明放进被忽略的目录。

### 运行链路

- Android：Flutter → Service MethodChannel → Kotlin 服务/JNI → 同进程 Go Core。`ServiceState` 门面下的原生状态机负责最新启动/停止意图；Flutter 的确认不意味着 VPN 已经建立。
- 桌面：Flutter → Rust 本地 IPC（Windows 命名管道，macOS/Linux Unix socket）→ 独立 Core 进程。进程归 `DesktopCoreLifecycle` 管理，通过 lease 获取和释放，UI 不能自行启动或杀进程。
- Rust 同时提供 QuickJS 脚本覆写引擎，因此 Android 也需要 Rust，并不是仅桌面需要。
- Windows/Linux Helper 可用于特权运行。普通代理不等同于 TUN 已可用；Helper 不就绪时可以降级为直接启动 Core。Linux AppImage 不支持这里的 TUN 授权流程。

协议、生命周期和安全约束详见 [.agents/architecture.md](.agents/architecture.md) 与 [.agents/rules.md](.agents/rules.md)。

## 4. 开发环境

下表是仓库配置的复现基准，不是所有工具的最低支持版本。升级时同时核对 CI、依赖解析和原生构建。

| 工具 | 当前基准 | 事实来源 |
| --- | --- | --- |
| Flutter | `3.47.1` | `.github/workflows/build.yaml` |
| Dart | 包语言下限 `>=3.10.0 <4.0.0`；实际 SDK 随 Flutter | `pubspec.yaml` |
| Go | CI 使用 `1.26.4` | `.github/workflows/build.yaml` |
| Rust | Rust API 固定 `1.95.0`，需 rustup、cargo、rustc | `plugins/rust_api/rust/rust-toolchain.toml` |
| Android | JDK 17、SDK 36、NDK `28.2.13676358`（r28c） | `android/app/build.gradle.kts`、`android/gradle/libs.versions.toml`、CI |

平台额外要求：

- **Windows**：Visual Studio 的 C++ 桌面开发工具（含 MSVC、Windows SDK、CMake），以及项目打包要求的 GCC、Inno Setup。在原生 Windows 环境构建 Windows 应用。
- **macOS**：Xcode 和命令行工具，按 Flutter 环境检查准备 CocoaPods；DMG 打包还需要 Node.js/npm 和 `appdmg`。
- **Linux**：Flutter 的 Linux 桌面编译环境（clang、CMake、Ninja、GTK 等）。`setup.dart` 的依赖安装针对 Debian/Ubuntu 的 `apt`/`dpkg`，并非通用 Linux 包管理器。
- **Android**：Android SDK、NDK 和已接受的 SDK 许可证；按 `flutter doctor -v` 处理 Java/SDK 路径。NDK 由 Flutter 提供给 build hook，不再要求手动设置 `ANDROID_NDK`。

## 5. 首次拉取与开发运行

首次获取源码并进入项目目录（已有工作副本跳过 clone 和 cd，从项目根目录继续）：

```bash
git clone --recurse-submodules https://github.com/ani13-teach/FlClash.git
cd FlClash
git submodule update --init --recursive
flutter doctor -v
flutter pub get
flutter devices
flutter run -d <设备ID> --dart-define=APP_ENV=dev
```

把 `<设备ID>` 换成 `flutter devices` 列出的值。桌面目标只能在对应宿主系统构建；Android 可从 Windows、macOS、Linux 构建。初次运行需要下载 Dart/Go/Rust 依赖并编译原生库，不是纯 Flutter 工程。

### Android 配置和签名

Gradle 启用了 Google Services 与 Crashlytics 插件。仓库跟踪了 `android/app/google-services.json`；替换成自己的 Firebase 项目时，配置的 Android 客户端包名必须与所构建变体匹配。CI 正式发布会注入自己的配置。不要把私钥、签名密码或个人服务凭据放进说明和提交。

正式签名需要 `android/app/keystore.jks`，以及 `android/local.properties` 中的 `storePassword`、`keyAlias`、`keyPassword`。**不需要正式签名密钥也可以构建本地 release APK**：缺少完整签名配置时，现有 Gradle 会使用 debug 签名和 `com.follow.clash.dev` 包名，而非正式的 `com.follow.clash`。

`--env stable` 只选择应用环境，不会自动赋予正式签名。签名不一致的已有开发包可能无法直接覆盖安装；先备份并检查签名，不要为了安装成功直接卸载用户数据。

## 6. 构建与打包

### 打包版本规则

- 相比上一次打包，只要源码有改动，下一次打包前必须升级 `pubspec.yaml` 中的 `version`，不得沿用上一次的完整版本号；本地测试包和正式发布包均适用。
- 按最小单位升级：默认只递增 `+` 后的构建号，使用 `bash tool/bump_version.sh minor`（按现有日期与当日序号规则生成），并确保新构建号大于上一次打包的构建号；不额外提升应用版本的主版本、次版本或修订号，除非发布要求明确需要。
- 源码未变化时，重复打包不强制升级版本号。

普通 `flutter build` 会触发原生 build hook；`setup.dart` 在此基础上调用分发工具，生成安装包/压缩包。

```bash
dart setup.dart --help
dart setup.dart android --arch arm64 --env dev
dart setup.dart windows --targets zip --env dev
dart setup.dart linux --targets deb --env dev
dart setup.dart macos --env dev
```

只执行与你的宿主平台匹配的桌面命令。省略平台时使用当前宿主；`--arch` 仅用于 Android，可选 `arm`、`arm64`、`amd64`。省略 `--env` 默认是 `pre`。

| 平台 | 默认打包目标 |
| --- | --- |
| Android | `apk`（按 ABI 拆分） |
| Windows | `exe,zip` |
| Linux | `deb,appimage,rpm` |
| macOS | `dmg`（宿主架构） |

重要副作用和输出：

- `setup.dart` 会重写本地 `env.json`，联网全局激活固定版本的 `chen08209/flutter_distributor` fork；需要 Dart pub 全局可执行目录在 `PATH` 中。
- Linux 会通过 `sudo apt-get` 安装缺失依赖，并在缺失时下载 AppImage 工具到 `/usr/local/bin/appimagetool`；macOS 会尝试全局安装 `appdmg`。不想自动改变系统环境时，先准备好依赖，或仅使用对应的 `flutter build`。
- 安装包输出在 `dist/`；直接 Flutter APK 输出通常在 `build/app/outputs/flutter-apk/`。Go/Helper 中间产物在 `libclash/`，Android Core 库和头文件还会复制进 `android/core/src/main/`。
- 必须保持 `pubspec.yaml` 中 `setup`、`rust_api` 的 `build_assets: true` 才能打包。测试临时关闭后需要恢复，否则会缺库或携带旧 Core；`setup.dart` 会拒绝这样的打包。
- 原生 hook 记录在 `.dart_tool/setup_build_cache/hook.log`。删除 `.dart_tool/setup_build_cache/` 会强制重建，不能解决缺工具链或错误签名的问题。

完整命令、原生验证和发布流程见 [.agents/commands.md](.agents/commands.md)。推送 `v*` 标签会触发发布链路，不是本地试验命令；未明确准备发布时不要运行带 `--push` 的 release 命令。

## 7. 修改与测试

最小常用检查：

```bash
flutter pub get
flutter analyze --no-fatal-infos
flutter test --reporter expanded
```

测试也可能触发原生构建，需要 Go/Rust 工具链。根包用 `flutter test`，不是 `dart test`；只有独立纯 Dart 包 `plugins/setup/setup_hooks/` 使用自己的 `dart test`。根包测试不自动发现嵌套插件或 Android/Go/Rust 测试。

按修改区域进一步检查：

| 修改范围 | 操作入口 |
| --- | --- |
| 模型、provider、数据库 | `dart run build_runner build --delete-conflicting-outputs`，再跑相关 `test/` 测试 |
| 翻译 | 修改四份 `arb/intl_*.arb`，运行 `dart run intl_utils:generate` |
| 本地 Flutter 插件 | `bash tool/check_plugins.sh` |
| Core/Sticky | 在 `core/` 运行 `CGO_ENABLED=0 go test .` 和 `CGO_ENABLED=0 go vet .`；根目录运行 `node --test tool/sticky_override_test.cjs` |
| Rust | `cargo test --manifest-path plugins/rust_api/rust/Cargo.toml` 与 `cargo test --manifest-path services/helper/Cargo.toml` |
| Android 生命周期 | JDK 17 下编译/测试相关模块，再做真机或模拟器验证 |

不要手改生成文件；遵守统一 lint、生命周期归属和 Conventional Commits。格式、覆盖率、平台专项门槛以 [命令说明](.agents/commands.md) 和 CI 为准。单元测试不能证明 Android 锁屏、VPN 授权、系统回收、Windows 服务权限等真实行为已经正常。

## 8. 数据、日志与排错

应用数据目录由 `path_provider.getApplicationSupportDirectory()` 决定，见 [lib/common/path.dart](lib/common/path.dart)，不是仓库根目录。重要数据包括 `database.sqlite`、配置和订阅文件、`scripts/`；Sticky 选择另存在 `sticky-selections.json`，按实际配置内容隔离。不要把各平台的数据路径硬编码为同一个目录。

| 现象 | 先检查 |
| --- | --- |
| 找不到 Core 源码或 Go 本地模块 | `git submodule status`，初始化 `core/Clash.Meta` |
| Flutter 测试开始编译 Go/Rust | 这是 build hook 的正常行为；查看工具链与 `hook.log` |
| Android 找不到 NDK/clang/libclang | SDK 下是否安装了指定 NDK，Flutter 是否识别到正确 SDK/JDK |
| APK 无法覆盖安装 | 包名、签名和已有安装是否一致；`.dev` 包不是正式包 |
| Core 正常但 TUN 不工作 | 权限和 Helper 是否就绪；降级直接启动不保证 TUN |
| Sticky 没有切换或 UI 暂时未更新 | 修改版 Core、精确的 `[Sticky] ` 前缀、直接节点候选、探测日志；UI 前台同步不是健康探测 |

反馈问题时附上系统/架构、应用版本或提交号、最短复现步骤、脱敏日志、失败命令。不要公开订阅链接中的 token、节点密码、WebDAV 凭据、签名密码或完整个人配置。

## 9. 文档导航和维护

- [README.md](README.md) / [README_zh_CN.md](README_zh_CN.md)：产品简介、上游下载和开发入口。
- [AGENTS.md](AGENTS.md)：自动化维护入口和最高优先级规则。
- [.agents/project.md](.agents/project.md)：版本事实来源、依赖 fork 和工具链。
- [.agents/architecture.md](.agents/architecture.md)：架构与所有权。
- [.agents/commands.md](.agents/commands.md)：开发、测试、打包、CI、发布命令。
- [.agents/rules.md](.agents/rules.md)：编码、生成代码、安全、测试、提交约束。
- [.agents/skills.md](.agents/skills.md)：UI、翻译、provider 测试、Core、Sticky 专项维护指引。
- [tool/sticky-failover.md](tool/sticky-failover.md)：本分支特色功能的使用与限制。
- [LICENSE](LICENSE)：GPL-3.0 许可证；再分发需遵守许可证和依赖各自的授权。

变更工具链、打包参数、协议、数据库或生命周期时同步更新相应说明。版本以配置文件和锁文件为事实来源，不要把某次升级遇到的依赖限制永久当成当前约束。
