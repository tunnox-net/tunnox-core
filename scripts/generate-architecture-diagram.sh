#!/bin/bash

# 生成架构分层图的PNG版本
# 需要安装 mermaid-cli: npm install -g @mermaid-js/mermaid-cli

echo "正在生成架构分层图..."

# 检查是否安装了 mermaid-cli
if ! command -v mmdc &> /dev/null; then
    echo "错误: 未找到 mermaid-cli"
    echo "请先安装: npm install -g @mermaid-js/mermaid-cli"
    exit 1
fi

# 创建输出目录
mkdir -p docs/images

# 生成架构分层图
mmdc -i docs/architecture-layers.mmd -o docs/images/architecture-layers.png -b transparent

if [ $? -eq 0 ]; then
    echo "✅ 架构分层图生成成功: docs/images/architecture-layers.png"
else
    echo "❌ 架构分层图生成失败"
    exit 1
fi

echo "完成！" 