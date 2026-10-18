<div>

[**简体中文**](README_zh_CN.md)

</div>

## FlClash

[![Downloads](https://img.shields.io/github/downloads/chen08209/FlClash/total?style=flat-square&logo=github)](https://github.com/chen08209/FlClash/releases/)[![Last Version](https://img.shields.io/github/release/chen08209/FlClash/all.svg?style=flat-square)](https://github.com/chen08209/FlClash/releases/)[![License](https://img.shields.io/github/license/chen08209/FlClash?style=flat-square)](LICENSE)

[![Channel](https://img.shields.io/badge/Telegram-Channel-blue?style=flat-square&logo=telegram)](https://t.me/FlClash)

A multi-platform proxy client based on ClashMeta, simple and easy to use, open-source and ad-free.

This repository is the `ani13-teach/FlClash` fork and includes Sticky proxy failover. Download links and badges below
refer to upstream. See the [project and development guide (中文)](PROJECT.md), [Sticky usage and limitations](tool/sticky-failover.md),
and [maintenance entry point](AGENTS.md) for this checkout.

<p align="center">
    <picture>
        <source media="(prefers-color-scheme: dark)" srcset="snapshots/preview-dark.png">
        <img alt="FlClash on desktop and mobile" src="snapshots/preview.png" width="90%">
    </picture>
</p>

## Features

✈️ Multi-platform: Android, Windows, macOS and Linux

💻 Adaptive multiple screen sizes, Multiple color themes available

💡 Based on Material You Design, [Surfboard](https://github.com/getsurfboard/surfboard)-like UI

☁️ Supports data sync via WebDAV

✨ Support subscription link, Dark mode

## Use

### Linux

⚠️ Make sure to install the following dependencies before using them

   ```bash
    sudo apt-get install libayatana-appindicator3-dev
   ```

### Android

Support the following actions

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

## Build

Install Flutter, Go, and Rust/rustup plus the target's native toolchain. Release CI uses Flutter `3.47.1` and Go
`1.26.4`; the Rust API pins Rust `1.95.0`. Android needs JDK 17, SDK 36, and NDK `28.2.13676358`. Flutter supplies the
NDK compiler to the build hook; no manual `ANDROID_NDK` setting is required. See the [development guide](PROJECT.md)
for platform prerequisites, Android configuration/signing, and troubleshooting.

From the repository root:

```bash
git submodule update --init --recursive
flutter doctor -v
flutter pub get
```

Run the command for your target; desktop builds require the matching host OS:

```bash
dart setup.dart android --arch arm64 --env dev
dart setup.dart windows --env dev
dart setup.dart linux --env dev
dart setup.dart macos --env dev
```

Packages go to `dist/`. The script rewrites local `env.json`, globally activates the pinned packaging tool, and may
install system packaging dependencies on Linux/macOS. `--env` defaults to `pre`; `--env stable` does not configure
release signing. Without a complete Android release signing configuration, local release APKs use a debug key and
`com.follow.clash.dev`. Keep both native `build_assets` hooks enabled when building an app.

For development and checks:

```bash
flutter run --dart-define=APP_ENV=dev
flutter analyze --no-fatal-infos
flutter test --reporter expanded
```

Tests can trigger native builds too. See [command reference](.agents/commands.md) for code generation, focused tests,
CI gates, and release procedures; see [architecture](.agents/architecture.md) for component ownership.

## Star

The easiest way to support developers is to click on the star (⭐) at the top of the page.

<p style="text-align: center;">
    <a href="https://api.star-history.com/svg?repos=chen08209/FlClash&Date">
        <img alt="start" width=50% src="https://api.star-history.com/svg?repos=chen08209/FlClash&Date"/>
    </a>
</p>
