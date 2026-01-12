# Tunnox Core AAR 构建指南

本文档说明如何在 Linux/macOS 环境下构建 tunnox-core.aar，该 AAR 已集成 tun2socks 功能。

## 环境要求

### 必需工具
- **Go 1.19+**: `go version`
- **Android SDK**: 设置 `ANDROID_HOME` 环境变量
- **Android NDK**: 设置 `ANDROID_NDK_HOME` 或放在 `$ANDROID_HOME/ndk/` 下
- **gomobile**: 用于将 Go 代码编译为 Android AAR

### 环境变量
```bash
export ANDROID_HOME=/path/to/android-sdk
export ANDROID_NDK_HOME=$ANDROID_HOME/ndk/27.0.12077973  # 或最新版本
export PATH=$PATH:$ANDROID_HOME/platform-tools:$ANDROID_HOME/cmdline-tools/latest/bin
```

## 构建步骤

### 1. 安装 gomobile

```bash
go install golang.org/x/mobile/cmd/gomobile@latest
gomobile init
```

### 2. 确保依赖已同步

```bash
cd tunnox-core
go mod tidy
```

### 3. 运行构建脚本

```bash
./build-mobile.sh
```

构建脚本会：
- 自动检测 Android SDK 和 NDK 路径
- 编译 `mobile` 包为 Android AAR
- 输出到 `../tunnox-tv/app/libs/tunnox-core.aar`
- 支持架构：arm64-v8a, armeabi-v7a, x86, x86_64

### 4. 验证构建结果

```bash
ls -lh ../tunnox-tv/app/libs/tunnox-core.aar
unzip -l ../tunnox-tv/app/libs/tunnox-core.aar | grep libgojni.so
```

应该看到：
- AAR 文件大小约 15-25 MB
- 包含所有架构的 `libgojni.so`

## 常见问题

### 问题 1: gomobile not found
```bash
# 确保 Go bin 目录在 PATH 中
export PATH=$PATH:$HOME/go/bin
```

### 问题 2: ANDROID_NDK_HOME not set
```bash
# 列出可用的 NDK 版本
ls $ANDROID_HOME/ndk/

# 设置环境变量
export ANDROID_NDK_HOME=$ANDROID_HOME/ndk/<version>
```

### 问题 3: gomobile init 失败
```bash
# 清理缓存重试
rm -rf ~/.gomobile
gomobile init
```

### 问题 4: 编译超时或内存不足
```bash
# 增加 Go 编译并行度
export GOMAXPROCS=4

# Linux 上增加内存限制
ulimit -v unlimited
```

## 手动构建（不使用脚本）

```bash
cd tunnox-core

# 编译 AAR
gomobile bind \
    -target=android \
    -o ../tunnox-tv/app/libs/tunnox-core.aar \
    -androidapi 21 \
    -ldflags="-s -w" \
    -v \
    ./mobile
```

参数说明：
- `-target=android`: 目标平台
- `-androidapi 21`: 最低 Android API 级别 (Android 5.0)
- `-ldflags="-s -w"`: 去除调试信息，减小体积
- `./mobile`: 编译 mobile 包

## 架构说明

### 核心功能 (mobile/client.go)
- `TunnoxMobileClient`: Tunnox 客户端封装
- 支持 TCP/WebSocket/KCP/QUIC 协议
- SOCKS5 映射管理

### Tun2socks 功能 (mobile/tun2socks.go)
- `Tun2SocksEngine`: tun2socks 引擎封装
- VPN fd → SOCKS5 代理转发
- 集成自 [xjasonlyu/tun2socks](https://github.com/xjasonlyu/tun2socks)

### 统一编译
两个功能使用同一个 Go module，编译到同一个 `libgojni.so`，避免 JNI 方法冲突。

## Linux 特定注意事项

### Ubuntu/Debian
```bash
# 安装必要的包
sudo apt-get update
sudo apt-get install -y build-essential git wget unzip

# 下载 Android SDK (Command Line Tools)
wget https://dl.google.com/android/repository/commandlinetools-linux-<version>_latest.zip
unzip commandlinetools-linux-*_latest.zip -d $HOME/android-sdk
export ANDROID_HOME=$HOME/android-sdk

# 安装 SDK 组件
$ANDROID_HOME/cmdline-tools/bin/sdkmanager --sdk_root=$ANDROID_HOME \
    "platform-tools" "platforms;android-35" "ndk;<version>"
```

### 权限问题
```bash
# 确保脚本有执行权限
chmod +x build-mobile.sh

# 如果遇到文件访问错误
sudo chown -R $USER:$USER .
```

### 编译优化
```bash
# 使用多核编译
export GOMAXPROCS=$(nproc)

# 禁用 CGO（如果不需要 C 绑定）
export CGO_ENABLED=1  # gomobile 需要 CGO
```

## 在 tunnox-tv 中使用

构建完成后，tunnox-tv 应用会自动使用新的 AAR：

```kotlin
// app/build.gradle.kts
dependencies {
    implementation(files("libs/tunnox-core.aar"))
}

// Kotlin 代码中调用
import mobile.Tun2SocksEngine
import mobile.TunnoxMobileClient

// 创建 tun2socks 引擎
val engine = mobile.Mobile.newTun2SocksEngine()
engine.start(vpnFd.fd.toLong(), "127.0.0.1:1080", 1500)
```

## 故障排查

### 查看详细日志
```bash
./build-mobile.sh 2>&1 | tee build.log
```

### 检查 Go 环境
```bash
go env
which gomobile
gomobile version
```

### 验证 AAR 内容
```bash
# 解压 AAR 查看结构
unzip -l ../tunnox-tv/app/libs/tunnox-core.aar

# 检查 JNI 库
unzip -l ../tunnox-tv/app/libs/tunnox-core.aar | grep "\.so$"

# 检查 Java 类
unzip -p ../tunnox-tv/app/libs/tunnox-core.aar classes.jar | jar tf | grep mobile
```

## 更新依赖

当需要更新 tun2socks 版本时：

```bash
cd tunnox-core
go get -u github.com/xjasonlyu/tun2socks/v2@latest
go mod tidy
./build-mobile.sh
```

## CI/CD 集成

示例 GitHub Actions 配置：

```yaml
- name: Setup Go
  uses: actions/setup-go@v4
  with:
    go-version: '1.21'

- name: Setup Android SDK
  uses: android-actions/setup-android@v2

- name: Install gomobile
  run: |
    go install golang.org/x/mobile/cmd/gomobile@latest
    gomobile init

- name: Build AAR
  run: |
    cd tunnox-core
    ./build-mobile.sh
```

## 参考链接

- [gomobile 官方文档](https://pkg.go.dev/golang.org/x/mobile/cmd/gomobile)
- [tun2socks 项目](https://github.com/xjasonlyu/tun2socks)
- [Android NDK 下载](https://developer.android.com/ndk/downloads)
