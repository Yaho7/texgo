# texgo

[English](./README.md)

`texgo` 是一个用于 LaTeX 项目的 Go 命令行工具。它通过缓存已转换的图片 PDF，减少 LaTeX 在重复编译中的图片处理开销，同时自动识别主 `.tex` 文件并调用 `latexmk` 完成编译。

## 功能

- 使用单个 `texgo` 命令构建 LaTeX 项目。
- 将支持的图片转换一次并复用 PDF 缓存，显著缩短后续重复编译时间。
- 自动识别 `main.tex`、`paper.tex`、`manuscript.tex`、`thesis.tex` 等常见主文件。
- 解析本地 `\includegraphics{...}` 引用并发现图片目录。
- 通过 GraphicsMagick 将支持的图片转换为缓存 PDF。
- 使用 `.texgo.conf` 保存项目配置。
- 使用 Go 构建单文件二进制，便于分发。

## 环境要求

运行时依赖：

- `latexmk`
- `latexmk` 支持的 LaTeX 引擎：`xelatex`、`pdflatex` 或 `lualatex`
- GraphicsMagick 提供的 `gm`，仅在启用图片转换时需要

源码构建依赖：

- Go，仅在从源码构建时需要

## 安装

安装最新发布版：

```bash
curl -fsSL https://raw.githubusercontent.com/Yaho7/texgo/main/install.sh | bash -s -- --yes
```

## 快速开始

首次配置项目：

```bash
texgo setup
```

构建项目：

```bash
texgo
```

在 LaTeX 项目目录中，`texgo` 等价于 `texgo build`。

## 命令

```bash
texgo                    # 构建当前项目
texgo setup              # 交互式创建或更新 .texgo.conf
texgo build              # 构建当前项目
texgo build main.tex     # 构建指定 TeX 文件
texgo images             # 只转换并缓存图片
texgo clean              # 删除构建目录
texgo clean --figures    # 删除构建产物和图片 PDF 缓存
texgo doctor             # 检查外部依赖命令
texgo init my-paper      # 创建最小示例项目
```

常用参数：

```text
--project-dir DIR
--figures-dir DIR
--pdf-dir DIR
--build-dir DIR
--tex-file FILE
--engine xelatex|pdflatex|lualatex
--no-images
```

## 配置

`texgo setup` 会在项目根目录写入 `.texgo.conf`。

示例：

```conf
tex_file=paper.tex
engine=xelatex
build_dir=build
figures_dir=
convert_images=1
```

优先级：

```text
命令行参数 > .texgo.conf > 自动检测默认值
```

## 图片转换

启用图片转换时，`texgo` 会将图片目录中的支持格式转换为 PDF，并保存到该图片目录下的 `pdf/` 子目录。后续构建会直接复用这些缓存，避免 LaTeX 反复处理大量图片资源；对于图片较多的论文、报告和毕业设计，这通常可以大幅缩短编译时间。

支持的源文件格式：

```text
png, jpg, jpeg, gif, tif, tiff, bmp, svg
```

当源图片比缓存 PDF 更新时，缓存会自动刷新。源图片不存在时，对应的过期 PDF 缓存会被删除。

## 开发

运行测试：

```bash
go test ./...
bash tests/install_release_test.sh
```

构建本地二进制：

```bash
./scripts/build-binary.sh --output /tmp/texgo
```

项目结构：

```text
cmd/texgo/              Go CLI 源码和测试
.github/workflows/      多平台发布构建
scripts/                构建辅助脚本
install.sh              发布版安装和源码构建辅助脚本
```
