#!/bin/bash
# build-mobile.sh - 编译 tunnox-core mobile 包为 Android AAR

set -e

echo "🔨 Building Tunnox Core Mobile AAR..."

# 检查环境变量
if [ -z "$ANDROID_HOME" ]; then
    echo "⚠️  ANDROID_HOME not set, trying to detect..."

    # 尝试自动检测 Android SDK 路径
    if [ -d "$HOME/Library/Android/sdk" ]; then
        export ANDROID_HOME="$HOME/Library/Android/sdk"
        echo "✅ Found ANDROID_HOME: $ANDROID_HOME"
    elif [ -d "$HOME/Android/Sdk" ]; then
        export ANDROID_HOME="$HOME/Android/Sdk"
        echo "✅ Found ANDROID_HOME: $ANDROID_HOME"
    else
        echo "❌ ANDROID_HOME not found. Please set it manually:"
        echo "   export ANDROID_HOME=/path/to/android-sdk"
        exit 1
    fi
fi

if [ -z "$ANDROID_NDK_HOME" ]; then
    echo "⚠️  ANDROID_NDK_HOME not set, trying to detect..."

    # 尝试查找最新的 NDK 版本
    if [ -d "$ANDROID_HOME/ndk" ]; then
        NDK_VERSION=$(ls -1 "$ANDROID_HOME/ndk" | tail -1)
        if [ -n "$NDK_VERSION" ]; then
            export ANDROID_NDK_HOME="$ANDROID_HOME/ndk/$NDK_VERSION"
            echo "✅ Found ANDROID_NDK_HOME: $ANDROID_NDK_HOME"
        fi
    fi

    if [ -z "$ANDROID_NDK_HOME" ]; then
        echo "❌ ANDROID_NDK_HOME not found. Please install Android NDK:"
        echo "   Android Studio > SDK Manager > SDK Tools > NDK"
        exit 1
    fi
fi

# 检查 Go 版本
GO_VERSION=$(go version | awk '{print $3}')
echo "📦 Go version: $GO_VERSION"

# 检查 gomobile 是否安装
if ! command -v gomobile &> /dev/null; then
    echo "📥 Installing gomobile..."
    go install golang.org/x/mobile/cmd/gomobile@latest

    echo "🔧 Initializing gomobile..."
    gomobile init
fi

# 输出目录
OUTPUT_DIR="../tunnox-tv/app/libs"
mkdir -p "$OUTPUT_DIR"

# 编译 AAR
echo "🔨 Compiling AAR..."
gomobile bind \
    -target=android \
    -o "$OUTPUT_DIR/tunnox-core.aar" \
    -androidapi 21 \
    -ldflags="-s -w" \
    -v \
    ./mobile

# 检查编译结果
if [ -f "$OUTPUT_DIR/tunnox-core.aar" ]; then
    AAR_SIZE=$(du -h "$OUTPUT_DIR/tunnox-core.aar" | awk '{print $1}')
    echo ""
    echo "✅ Build complete!"
    echo "   📦 Output: $OUTPUT_DIR/tunnox-core.aar"
    echo "   📏 Size: $AAR_SIZE"
    echo ""

    # 验证 AAR 内容
    echo "📋 AAR Contents:"
    unzip -l "$OUTPUT_DIR/tunnox-core.aar" | grep -E "\.so$|classes.jar" || true
    echo ""

    # 提取架构信息
    echo "🏗️  Supported Architectures:"
    unzip -l "$OUTPUT_DIR/tunnox-core.aar" | grep -oE "(arm64-v8a|armeabi-v7a|x86|x86_64)" | sort -u
    echo ""
else
    echo "❌ Build failed!"
    exit 1
fi

echo "🎉 Done!"
