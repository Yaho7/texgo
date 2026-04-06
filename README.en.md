## 📖 选择语言 | Select Language

- [English](./README.en.md)
- [简体中文](./README.md)
  
# Significantly Improve LaTeX Compilation Speed for Windows, MacOS, Linux


## Introduction

As the LaTeX manuscript grows longer and more images are added, the compilation speed becomes increasingly slower—especially when a large number of PNG or JPEG images are included, each compilation may take 60 seconds or even longer.

This happens because LaTeX needs to compress and convert these image formats into PDF during compilation before performing typesetting, which significantly slows down the process.

With the setup in this tutorial, you can **reduce the complete paper compilation time to under 5 seconds**.

This tutorial is divided into two stages:

- Configuring VS Code's `settings.json`
  - Setting up the `latexmk` compilation tool
  - Configuring scripts for automatic image conversion to PDF

## Implementation Steps

0. Install ImageMagick tool (see appendix at the end)
1. Clone this project
   ```
   git clone https://github.com/Yaho7/latex-fastbuild.git
   ```
2. Install dependencies
   - Ensure ImageMagick is installed locally (on Mac use `brew install imagemagick`)
   - Install LaTeX and VS Code
3. Open this project in VS Code
   - The settings.json is already configured by default
   - Open and edit template/manuscript.tex
4. Choose the compilation recipe
   - Right-click in VS Code and select "Build with LaTeX Workshop" or select the latexmk recipe from the command palette

## Project Layout

```
scripts/                # Scripts
template/               # Template content and assets
   manuscript.tex
   bibliography/
   figures/
   styles/
build/                  # Build outputs
```

## How It Works

### 1. Using `latexmk` for Smart Compilation

Compared to traditional `xelatex` manual compilation, `latexmk` offers the following advantages:

- **Automatic Multi-round Compilation**: Automatically detects changes in `.aux`, `.toc`, `.bbl` and other files, and automatically calls the compiler multiple times until output stabilizes.
- **Automatic Reference Processing**: When citations change, it automatically runs BibTeX or Biber.
- **Selective Compilation**: If parts of the content haven't changed, it won't recompile them, significantly improving speed.

### 2. Converting Images to PDF with Long-term Caching

Our custom script automatically handles the following logic:

- Checks if images already have corresponding PDF files during compilation
- If yes, skips them
- If no, automatically converts PNG, JPEG, and other formats to PDF and saves them
- Conversion results are cached long-term, eliminating the need for repeated processing in subsequent compilations, significantly speeding up the process

# Appendix

## **graphicsmagick Installation Guide**

### **macOS System**

#### **Install using Homebrew (Recommended)**

1. Open Terminal;

2. Enter the following command to install:

   ```
   brew install graphicsmagick
   ```

3. Verify the installation:

   ```
   gm convert -version
   ```

   > Note: On macOS, the command for GraphicsMagick is usually `gm`, not `convert`. `convert` is an ImageMagick command.

---

### **Windows System**

#### **Install using Installer (Recommended)**

1. Go to the [GraphicsMagick official download page](http://www.graphicsmagick.org/);
2. Under "Windows Packages," choose the appropriate version (usually Win64 dynamic at 16 bits per pixel);
3. Download and run the `.exe` installer;
4. During installation, check **Add application directory to your system path** (adds GraphicsMagick to the system environment variable);
5. Open Command Prompt (CMD) or PowerShell and enter:

   ```
   gm convert -version
   ```

   If the version information appears, the installation was successful.

---

### **Linux System**

#### **Install using Package Manager**

1. Open Terminal;
2. Enter the following command according to your distribution:

   - **Debian/Ubuntu:**
     ```
     sudo apt-get update
     sudo apt-get install graphicsmagick
     ```

   - **CentOS/Fedora:**
     ```
     sudo dnf install GraphicsMagick
     ```
     or
     ```
     sudo yum install GraphicsMagick
     ```

3. Verify the installation:

   ```
   gm convert -version
   ```

   If you see the version information, the installation was successful.

> If you encounter issues or are using another distribution, please refer to the [official installation documentation](http://www.graphicsmagick.org/INSTALL-unix.html).