# Project Context

FlClash is a multi-platform proxy client based on ClashMeta (mihomo), built with Flutter for Android, Windows, macOS,
and Linux. It uses Material You styling with a Surfboard-like UI.

This checkout is the `ani13-teach/FlClash` fork of `chen08209/FlClash`. Download badges and package-manager links in the
READMEs refer to upstream, not necessarily this fork. The fork includes Sticky proxy failover in the Go wrapper and a
subscription override script; see [../tool/sticky-failover.md](../tool/sticky-failover.md). The user/developer overview is
[../PROJECT.md](../PROJECT.md).

## Version Sources

Use configuration files as the source of truth rather than historical upgrade notes:

| Setting | Current configuration | Source |
| --- | --- | --- |
| Application version | `version` field | `pubspec.yaml` |
| Release Flutter SDK | `3.47.1` | `.github/workflows/build.yaml` |
| Dart package language floor | `>=3.10.0 <4.0.0` | `pubspec.yaml` |
| CI Go toolchain | `1.26.4` | `.github/workflows/build.yaml` |
| Rust API toolchain | `1.95.0`, exact target list | `plugins/rust_api/rust/rust-toolchain.toml` |
| Android compile/target SDK | `36` | `android/gradle/libs.versions.toml` |
| Android NDK | `28.2.13676358` (r28c) | Android version catalog and CI |
| Android Java | `17` | `android/app/build.gradle.kts` and CI |

The Dart lower bound sets the package language version; it is not the Dart SDK installed on the machine. Do not raise it
just to match the current SDK. Local tools may differ, but use CI's pins for reproducible release builds; these pins are
not a claim about minimum compatible versions.

## Forked Dependencies

Three `pubspec.yaml` dependencies are pinned to `chen08209` forks: `window_manager` by tag, the other two by commit SHA.
They are maintained by the upstream FlClash account, not by the owner of this fork; advancing a pin needs verification of
both the fork changes and their FlClash call sites.

### window_manager

- Source: `chen08209/window_manager`, path `packages/window_manager`.
- Pin: `v0.5.1-flclash.3`; package version remains `0.5.1`.
- The documented fork requirements include Windows maximized hidden-titlebar work-area handling, Linux hidden-window
  geometry restoration, and additional Windows/macOS APIs (`setWindowCornerPreference`, `handleShouldTerminate`,
  `onWindowShouldTerminate`). Callers include `lib/manager/window_manager.dart` and `macos/Runner/AppDelegate.swift`.
- Check the complete diff at the pinned tag before advancing or replacing it; the newer fork tag is not equivalent to
  the published package merely because its package version matches. Removing the fork requires upstream to supply the
  behavior and APIs used here, or an explicit caller migration.

### launch_at_startup

- Source: `chen08209/launch_at_startup`, pin `e930ce65c5804343103447e01d39fccabedc8681`, version `0.5.1`.
- The fork accepts `win32_registry` 3.x and adapts the Windows API renames. FlClash also depends on `win32_registry: ^3.0.3`
  directly; the older published dependency constraint cannot co-resolve with it.
- Drop the fork only after a published replacement accepts the required registry version and startup behavior is tested.

### yaml_writer

- Source: `chen08209/yaml_writer`, pin `79c78a44ec9c2f5f5f97da1a65610c6fb2ead8b3`, version `2.1.0`.
- Adds quoting of map keys by the same validity rule already used for values, preventing invalid profile YAML.
- Drop the fork only when the replacement quotes those keys and profile serialization tests still pass.

`setup.dart` additionally activates `chen08209/flutter_distributor` at `v0.6.11-flclash.2` as a global packaging tool. It
is not a local package under `plugins/`. See [architecture.md](architecture.md) for its packaging constraints.

To re-verify a fork, compare the exact pinned checkout in the pub cache with the corresponding published package. Pub
cache paths vary by host (`PUB_CACHE` may override them); do not assume the Unix default on Windows.

## Dependency Upgrades

Current declared generator/runtime constraints include:

- `freezed: ^4.0.1` with `freezed_annotation: ^3.1.0`.
- `riverpod` and `flutter_riverpod: ^3.4.2`, `riverpod_annotation: ^4.0.6`, `riverpod_generator: ^4.0.8`.
- `build_runner: ^2.15.1`, `drift_dev: ^2.34.0`, `intl_utils: ^2.8.15`.

These are constraints, not necessarily exact resolved versions; inspect `pubspec.lock` for the latter. Earlier notes
about Freezed `3.2.6-dev.1`, a mandatory Dart `3.8.0` floor, or Riverpod `3.3.2` ceilings do not describe this checkout.
Do not downgrade current dependencies to satisfy those historical notes.

For a new upgrade, run `flutter pub outdated` and verify resolution under the chosen Flutter SDK. Flutter-pinned test
packages and generator/analyzer constraints can limit upgrades, but only record a ceiling when the current resolver
actually demonstrates it. Treat Riverpod runtime/annotation/generator and Freezed runtime/generator as coordinated sets;
regenerate code, analyze, and run focused tests after changing them. Do not manually repair generated code to hide an
incompatible upgrade.

Dynamic color initialization is in `Bootstrap._initDynamicColor` (`lib/bootstrap.dart`), which converts a system primary
`TonalPalette` into the plain seeds the app consumes. The old `GlobalState._initDynamicColor`/`CorePalette` migration note
is obsolete; inspect the current implementation before changing the dependency or platform-channel decoding.

## Build Dependencies

All platforms need Flutter/Dart, Go, and Rust/rustup because native hooks compile the Core and the QuickJS/IPC library.
Windows and Linux also build the Rust Helper. Flutter builds and root tests can trigger these hooks.

- Windows: Flutter's Visual Studio C++ desktop toolchain plus GCC and Inno Setup for packaging.
- Linux: Flutter's desktop native toolchain plus the packages installed by `setup.dart` (`ninja-build`, `libgtk-3-dev`,
  `libayatana-appindicator3-dev`, `libsecret-1-dev`, `locate`, `rpm`, `patchelf`, `libfuse2` or `libfuse2t64`) and AppImage
  tooling. The automatic installer targets Debian/Ubuntu; the runtime tray dependency alone is not a full build setup.
- macOS: Xcode/Flutter native prerequisites and Node.js/npm with `appdmg` for DMG packaging.
- Android: JDK 17, configured Android SDK/NDK, SDK licenses, and Google Services configuration. Flutter supplies the NDK
  compiler to the hook; setting `ANDROID_NDK` is not the current build interface.

See [commands.md](commands.md) for commands and [../PROJECT.md](../PROJECT.md) for first-run instructions, local signing
fallback, build side effects, output directories, and troubleshooting.
