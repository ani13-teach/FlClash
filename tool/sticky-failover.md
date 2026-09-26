# Sticky Failover 首版

本功能只管理名称以 `[Sticky] `（含空格）开头的 `select` 组。不修改 Mihomo 子模块，不需要 Root 或 external-controller。

## 使用

把 `tool/sticky_override.js` 完整粘贴到订阅的脚本覆写中，再应用配置。它生成全局和国家粘滞组，过滤名称能识别出的香港节点，并关闭 provider 的周期全量健康检查。香港识别依赖名称，不是 IP 地理位置验证。原有规则匹配顺序保留，普通代理目标统一接到“节点选择”；纯 DIRECT/REJECT 策略保留。

该脚本必须配合此修改版 Core；官方 FlClash 的 select 组不会自动故障转移。原来的 `C:/Users/jch/flclash_override.js` 未修改。

## 行为

- 目标检测周期 1 秒，实际请求超时 2 秒，失败一次扫描备用节点。
- 固定地址 `https://cp.cloudflare.com`，使用 Mihomo 的 URLTest；HTTP 请求成功且未超过总请求期限即健康，不表示所有网站或 UDP 都正常。
- 当前节点健康时只测当前节点；故障时并发探测组内其余直接节点，谁先确认健康就立即选谁，不等全部探测结束，也不按延迟排名。原节点恢复不会主动切回。
- 每组不重叠检测。同一节点实例的在途请求和一秒窗口内结果共享；不同 provider 的同名节点不会混用结果。
- Sticky 实际探测并发上限 16；大组或多个组同时故障时，其余探测最多排队 250 毫秒。排队资源不足不计节点失败，下轮重试；所有候选确实失败后等待 5 秒。无法保证 Android 锁屏时严格每秒执行。
- 所有命名粘滞组都会运行，并非只有入口当前选中的一个组。脚本中的组只含直接节点，不含嵌套组；DIRECT、REJECT 等不作为自动候选。手动选到这类成员时暂停该组探测，直到重新选回代理节点。
- 用户手动选择优先，旧扫描不能提交覆盖它。配置替换、provider 成员变更会使旧快照失效；停止和挂起取消请求，恢复后重建任务。
- 切换影响后续建立的连接，不强制断开所有既有 TCP/UDP 会话。

## 状态与生命周期

Core 将选择保存在应用数据目录的 `sticky-selections.json`，按实际应用的 `config.yaml` 内容 SHA256 隔离。同一配置重新应用或进程重启会恢复选择；配置内容变化视作另一个范围，不承诺延续原选择。移除的节点由 Selector 回退到现有成员。文件写入错误通过 Core 日志报告。

Flutter 对粘滞组以 Core 的 `now` 为准，前台每两秒同步一次，返回前台立即同步；这只是 UI 状态同步，不承担健康检测。引擎不在时 Go 仍由原生 Service 生命周期管理。整个应用进程被杀后，必须由原生服务恢复路径重建 Core；本功能不保证进程永不被系统回收。

禁用方式：去掉组名的 `[Sticky] ` 前缀并应用配置。普通 select 组保持原行为。

## 验证与打包

无需工具链的脚本测试：

```bash
node --test tool/sticky_override_test.cjs
```

工具链可用后需要完成：

```bash
cd core
gofmt -w sticky.go sticky_engine.go sticky_engine_test.go sticky_test.go common.go hub.go
CGO_ENABLED=0 go test .
CGO_ENABLED=0 go vet .
cd ..
flutter pub get
dart run build_runner build --delete-conflicting-outputs
dart format lib/models/common.dart lib/manager/core_manager.dart test/models/common_test.dart test/manager/core_manager_test.dart
flutter analyze --no-fatal-infos
flutter test test/models/common_test.dart test/manager/core_manager_test.dart test/core/protocol_contract_test.dart
flutter build apk --release --target-platform android-arm64 --split-per-abi --dart-define=APP_ENV=dev
```

Flutter 测试也会触发原生构建 hook；必要时按 `.agents/commands.md` 临时关闭测试用原生构建，打包前必须恢复，不能带着关闭的 hook 打包。构建依赖以项目配置为准：Flutter 3.47.1、Go 1.26.4、Rust 1.95.0，以及 Android NDK 28.2.13676358。

未配置 release 签名时，现有 Gradle 配置使用 debug 签名和 `com.follow.clash.dev` 包名，可与官方包并存，但可能与已有开发版冲突。不要删除已有安装来强行覆盖。

已安装 Flutter 3.47.1、Go 1.26.4、Rust 1.95.0。`CGO_ENABLED=0 go test .`、`go vet .`、定向 Flutter 测试与覆写脚本测试均已通过；Go Core 也已用 NDK 28.2 成功交叉编译为 Android arm64 共享库。arm64 release APK 已构建成功：`build/app/outputs/flutter-apk/app-arm64-v8a-release.apk`。已核对 APK 完整性、`arm64-v8a` ABI、Go/Rust/SQLite 原生库、`com.follow.clash.dev` 包名及 debug 签名。Android 真机行为尚未验证。
