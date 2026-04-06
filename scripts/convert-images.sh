#!/usr/bin/env bash

# ===============================
# 跨平台 GraphicsMagick 图片转 PDF 脚本
# 支持 Linux / macOS / Windows Git Bash / WSL
# 自动检查并安装 gm（GraphicsMagick）
# ===============================

SOURCE_DIR="$(pwd)/template/figures"
TARGET_DIR="$SOURCE_DIR/pdf"
EXTS=(png jpg jpeg gif tif tiff bmp svg)

# 检查 gm 是否可用，不可用则尝试自动安装
if ! command -v gm &> /dev/null; then
    echo "❌ 未检测到 GraphicsMagick (gm)，正在尝试自动安装..."

    # 检查操作系统类型
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        if command -v apt &> /dev/null; then
            sudo apt update && sudo apt install -y graphicsmagick
        elif command -v yum &> /dev/null; then
            sudo yum install -y GraphicsMagick
        elif command -v pacman &> /dev/null; then
            sudo pacman -Sy graphicsmagick
        else
            echo "⚠️ 未知 Linux 包管理器，请手动安装 GraphicsMagick。"
            exit 1
        fi
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        if command -v brew &> /dev/null; then
            brew install graphicsmagick
        else
            echo "⚠️ 未检测到 Homebrew，请先安装 Homebrew 后重试。"
            exit 1
        fi
    elif [[ "$OSTYPE" == "msys"* ]] || [[ "$OSTYPE" == "win32"* ]]; then
        echo "⚠️ Windows 用户请手动到 http://www.graphicsmagick.org/ 下载 GraphicsMagick，并确保 'gm' 命令可用。"
        exit 1
    else
        echo "⚠️ 未识别的操作系统类型，请手动安装 GraphicsMagick。"
        exit 1
    fi

    # 安装后重新检测
    if ! command -v gm &> /dev/null; then
        echo "❌ 自动安装 GraphicsMagick 失败，请手动安装。"
        exit 1
    fi
    echo "✅ GraphicsMagick 安装成功。"
fi

mkdir -p "$TARGET_DIR"

# 转换图片为 PDF
for ext in "${EXTS[@]}"; do
    find "$SOURCE_DIR" -maxdepth 1 -type f \( -iname "*.$ext" -o -iname "*.${ext^^}" \) | while IFS= read -r img; do
        filename=$(basename -- "$img")
        name="${filename%.*}"
        pdf="$TARGET_DIR/$name.pdf"

        if [ ! -f "$pdf" ] || [ "$img" -nt "$pdf" ]; then
            echo "📄 正在转换 $filename → $name.pdf"
            gm convert "$img" "$pdf"
        fi
    done
done

# 删除没有源图的 PDF
find "$TARGET_DIR" -maxdepth 1 -type f -iname "*.pdf" | while IFS= read -r pdf; do
    name=$(basename -- "$pdf" .pdf)

    found=0
    for ext in "${EXTS[@]}"; do
        if [ -f "$SOURCE_DIR/$name.$ext" ] || [ -f "$SOURCE_DIR/$name.${ext^^}" ]; then
            found=1
            break
        fi
    done

    if [ $found -eq 0 ]; then
        echo "🗑️ 源图片已删除，正在删除 PDF: $name.pdf"
        rm "$pdf"
    fi
done

echo "✅ 图片转换完成"