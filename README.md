  ## 📖 选择语言 | Select Language

  - [English](./README.en.md)
  - [简体中文](./README.md)

# 大幅提高 LaTeX 的编译速度, 适用于Windows,MacOS,Linux

  ## 前言

  Latex会随着文稿逐渐变长、图片增多，编译速度越来越慢——特别是当插入大量 PNG、JPEG 图片时，每次编译往往需要 60 秒甚至更久。

  这是因为 LaTeX 在编译过程中需要将这些图片格式压缩、转换为 PDF 后再进行排版处理，极大拖慢了速度。

  通过本教程的设置，你可以将完整论文的编译时间 **压缩至 5 秒以内**。

  本教程分为两个阶段：

  - 配置 VS Code 的 `settings.json`
    - 设置 `latexmk` 编译工具
    - 配置图片自动转换 PDF 的脚本


  # 实现步骤
  1. 安装graphicsmagick工具,请看文末附录
  2. 克隆本项目 
   - git clone https://github.com/Yaho7/latex-fastbuild.git
  3. 安装依赖
   - 确保本地已安装 graphicsmagick（Mac 上可使用 brew install graphicsmagick）
   - 安装好 LaTeX 及 VS Code
  4. 在 VS Code 中打开本项目
   - 默认已配置好 settings.json
   - 打开并编写 template/manuscript.tex
  5. 选择编译配方
   - 在 VS Code 中右键选择 “使用 LaTeX Workshop 编译”，或从命令面板选择 latexmk 配方

## 目录结构

```
scripts/                # 脚本
template/               # 模板正文与资源
  manuscript.tex
  bibliography/
  figures/
  styles/
build/                  # 编译产物
```


  ## 实现原理

  ### 1. 使用 `latexmk` 进行智能编译

  相比传统的 `xelatex` 手动编译方式，`latexmk` 具有以下优势：

  - **自动多轮编译**：自动检测 `.aux`、`.toc`、`.bbl` 等文件的变化，并自动调用编译器多次，直到输出稳定。
  - **自动处理参考文献**：当引用发生变化时，会自动运行 BibTeX 或 Biber。
  - **只编译必要部分**：如果部分内容未更改，它不会重复编译，从而显著提升速度。

  ### 2. 图片转换为 PDF 并长期缓存

  我们通过一个自定义脚本自动完成以下逻辑：

  - 编译时检查图片是否已有对应 PDF 文件；
  - 如有，则跳过；
  - 如无，则自动将 PNG、JPEG 等格式转换为 PDF 并保存；
  - 转换结果长期缓存，后续编译无需重复处理，显著加快速度。


# 附录

## **graphicsmagick 安装教程**

### **macOS 系统**

#### **使用 Homebrew 安装（推荐）**

1. 打开 Terminal（终端）；

2. 输入以下命令安装：

   ```
   brew install graphicsmagick
   ```

3. 验证是否安装成功：

   ```
   gm convert -version
   ```



---

### **Windows 系统**

#### **使用安装包安装（推荐）**

1. 打开 [GraphicsMagick 官方下载页面](http://www.graphicsmagick.org/)；
2. 在 Windows Packages 中选择合适的版本（一般为 Win64 dynamic at 16 bits per pixel）；
3. 下载并运行 `.exe` 安装程序；
4. 安装时勾选 **Add application directory to your system path**（将 GraphicsMagick 添加到系统环境变量）；
5. 打开 命令提示符（CMD）或 PowerShell，输入：

   ```
   gm convert -version
   ```

   如果出现版本信息，则说明安装成功。

---

### **Linux 系统**

#### **使用包管理器安装**

1. 打开终端；
2. 根据你的发行版，输入以下命令之一：

   - **Debian/Ubuntu：**
     ```
     sudo apt-get update
     sudo apt-get install graphicsmagick
     ```

   - **CentOS/Fedora：**
     ```
     sudo dnf install GraphicsMagick
     ```
     或
     ```
     sudo yum install GraphicsMagick
     ```

3. 验证是否安装成功：

   ```
   gm convert -version
   ```

   如果看到版本信息，则说明安装成功。

> 如果遇到问题或使用其他发行版，请参考 [官方安装文档](http://www.graphicsmagick.org/INSTALL-unix.html)。
