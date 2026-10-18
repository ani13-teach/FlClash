<div>

[**English**](README.md)

</div>

## FlClash

[![Downloads](https://img.shields.io/github/downloads/chen08209/FlClash/total?style=flat-square&logo=github)](https://github.com/chen08209/FlClash/releases/)[![Last Version](https://img.shields.io/github/release/chen08209/FlClash/all.svg?style=flat-square)](https://github.com/chen08209/FlClash/releases/)[![License](https://img.shields.io/github/license/chen08209/FlClash?style=flat-square)](LICENSE)

[![Channel](https://img.shields.io/badge/Telegram-Channel-blue?style=flat-square&logo=telegram)](https://t.me/FlClash)

基于ClashMeta的多平台代理客户端，简单易用，开源无广告。

本仓库是 `ani13-teach/FlClash` 分支，包含 Sticky 自动故障转移功能。下方下载链接和徽章指向上游版。
本项目的使用、目录结构、环境准备、构建、测试与排错见 [项目说明与开发指南](PROJECT.md)；
特色功能见 [Sticky 使用说明与限制](tool/sticky-failover.md)；维护入口见 [AGENTS.md](AGENTS.md)。

<p align="center">
    <picture>
        <source media="(prefers-color-scheme: dark)" srcset="snapshots/preview-dark.png">
        <img alt="FlClash on desktop and mobile" src="snapshots/preview.png" width="90%">
    </picture>
</p>

## Features

✈️ 多平台: Android, Windows, macOS and Linux

💻 自适应多个屏幕尺寸,多种颜色主题可供选择

💡 基本 Material You 设计, 类[Surfboard](https://github.com/getsurfboard/surfboard)用户界面

☁️ 支持通过WebDAV同步数据

✨ 支持一键导入订阅, 深色模式

## Use

### Linux

⚠️ 使用前请确保安装以下依赖

   ```bash
    sudo apt-get install libayatana-appindicator3-dev
   ```

### Android

支持下列操作

   ```bash
    com.follow.clash.action.START
    
    com.follow.clash.action.STOP
    
    com.follow.clash.action.TOGGLE
   ```

## Download

<a href="https://chen08209.github.io/FlClash-fdroid-repo/repo?fingerprint=789D6D32668712EF7672F9E58DEEB15FBD6DCEEC5AE7A4371EA72F2AAE8A12FD"><img alt="Get it on F-Droid" src="snapshots/get-it-on-fdroid.svg" width="200px"/></a> <a href="https://github.com/chen08209/FlClash/releases"><img alt="Get it on GitHub" src="snapshots/get-it-on-github.svg" width="200px"/></a>

### Homebrew

```bash
brew tap chen08209/tap
brew install --cask flclash
```

## 构建

安装 Flutter、Go、Rust/rustup 以及目标平台原生工具链。发布 CI 使用 Flutter `3.47.1`、Go `1.26.4`；
Rust API 固定 Rust `1.95.0`。Android 需要 JDK 17、SDK 36、NDK `28.2.13676358`。
Flutter 会把 NDK 编译器传给构建 hook，不再要求手动设置 `ANDROID_NDK`。
完整的平台前置条件、Android 配置和签名说明见 [开发指南](PROJECT.md)。

在仓库根目录执行：

```bash
git submodule update --init --recursive
flutter doctor -v
flutter pub get
```

按目标选择命令；桌面应用必须在对应系统上构建：

```bash
dart setup.dart android --arch arm64 --env dev
dart setup.dart windows --env dev
dart setup.dart linux --env dev
dart setup.dart macos --env dev
```

安装包输出到 `dist/`。脚本会重写本地 `env.json`、全局激活固定版本的打包工具，并可能在 Linux/macOS
安装系统打包依赖。省略 `--env` 默认是 `pre`；`--env stable` 不会自动配置正式签名。
Android 缺少完整 release 签名配置时，release APK 使用 debug 签名与 `com.follow.clash.dev` 包名。
构建应用时必须保持两个原生 `build_assets` hook 开启。

开发运行和常用检查：

```bash
flutter run --dart-define=APP_ENV=dev
flutter analyze --no-fatal-infos
flutter test --reporter expanded
```

测试也可能触发原生编译。代码生成、分模块测试、CI 和发布操作见 [命令说明](.agents/commands.md)，
组件职责见 [架构说明](.agents/architecture.md)。

## Star

支持开发者的最简单方式是点击页面顶部的星标（⭐）。

<p style="text-align: center;">
    <a href="https://api.star-history.com/svg?repos=chen08209/FlClash&Date">
        <img alt="start" width=50% src="https://api.star-history.com/svg?repos=chen08209/FlClash&Date"/>
    </a>
</p>
