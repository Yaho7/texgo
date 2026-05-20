package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("TEXGO_FAKE_TOOL") == "1" {
		os.Exit(runFakeTool())
	}
	os.Exit(m.Run())
}

func runFakeTool() int {
	switch filepath.Base(os.Args[0]) {
	case "gm":
		if len(os.Args) == 4 && os.Args[1] == "convert" {
			if err := os.MkdirAll(filepath.Dir(os.Args[3]), 0755); err != nil {
				return 1
			}
			src, err := os.Open(os.Args[2])
			if err != nil {
				return 1
			}
			defer src.Close()
			dst, err := os.Create(os.Args[3])
			if err != nil {
				return 1
			}
			defer dst.Close()
			if _, err := io.Copy(dst, src); err != nil {
				return 1
			}
			return 0
		}
		return 2
	case "latexmk":
		argsFile := os.Getenv("LATEXMK_ARGS_FILE")
		if argsFile == "" {
			return 1
		}
		content := strings.Join(os.Args[1:], "\n") + "\n"
		if err := os.WriteFile(argsFile, []byte(content), 0644); err != nil {
			return 1
		}
		return 0
	default:
		return 127
	}
}

func TestHelp(t *testing.T) {
	var out, err bytes.Buffer
	a := app{stdin: strings.NewReader(""), stdout: &out, stderr: &err}

	if runErr := a.run([]string{"--help"}); runErr != nil {
		t.Fatalf("run help: %v", runErr)
	}

	got := out.String()
	for _, want := range []string{"Usage: texgo", "setup", "build", "images"} {
		if !strings.Contains(got, want) {
			t.Fatalf("help output missing %q:\n%s", want, got)
		}
	}
}

func TestImagesConvertsAndPrunesCachedPDFs(t *testing.T) {
	project := realPath(t, t.TempDir())
	fakeBin := t.TempDir()
	installFakeTool(t, fakeBin, "gm")
	mkdirAll(t, filepath.Join(project, "figures", "pdf"))
	writeFile(t, filepath.Join(project, "figures", "logo.png"), "png")
	writeFile(t, filepath.Join(project, "figures", "pdf", "stale.pdf"), "stale")

	withEnv(t, "PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	withEnv(t, "TEXGO_FAKE_TOOL", "1")
	runInDir(t, project, func() {
		var out, stderr bytes.Buffer
		a := app{stdin: strings.NewReader(""), stdout: &out, stderr: &stderr}
		if err := a.run([]string{"images"}); err != nil {
			t.Fatalf("images: %v\nstderr=%s", err, stderr.String())
		}
	})

	assertFile(t, filepath.Join(project, "figures", "pdf", "logo.pdf"))
	assertNotFile(t, filepath.Join(project, "figures", "pdf", "stale.pdf"))
}

func TestImagesUsesUppercaseFiguresAndExistingPDFCacheDir(t *testing.T) {
	project := realPath(t, t.TempDir())
	fakeBin := t.TempDir()
	installFakeTool(t, fakeBin, "gm")
	writeFile(t, filepath.Join(project, "src", "manuscript.tex"), `\documentclass{article}\begin{document}\includegraphics{Plot.pdf}\end{document}`)
	mkdirAll(t, filepath.Join(project, "src", "Figures", "PDF"))
	writeFile(t, filepath.Join(project, "src", "Figures", "Plot.png"), "png")

	withEnv(t, "PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	withEnv(t, "TEXGO_FAKE_TOOL", "1")
	runInDir(t, project, func() {
		var out, stderr bytes.Buffer
		a := app{stdin: strings.NewReader(""), stdout: &out, stderr: &stderr}
		if err := a.run([]string{"images", "--tex-file", "src/manuscript.tex"}); err != nil {
			t.Fatalf("images: %v\nstdout=%s\nstderr=%s", err, out.String(), stderr.String())
		}
	})

	assertFile(t, filepath.Join(project, "src", "Figures", "PDF", "Plot.pdf"))
	assertNoChildDirNamed(t, filepath.Join(project, "src", "Figures"), "pdf")
}

func TestBuildWorksWithoutBundledTemplate(t *testing.T) {
	project := realPath(t, t.TempDir())
	fakeBin := t.TempDir()
	argsFile := filepath.Join(project, "latexmk.args")
	installFakeTool(t, fakeBin, "gm")
	installFakeTool(t, fakeBin, "latexmk")
	mkdirAll(t, filepath.Join(project, "assets"))
	writeFile(t, filepath.Join(project, "main.tex"), `\documentclass{article}\usepackage{graphicx}\begin{document}\includegraphics{assets/chart.png}\end{document}`)
	writeFile(t, filepath.Join(project, "assets", "chart.png"), "png")

	withEnv(t, "PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	withEnv(t, "TEXGO_FAKE_TOOL", "1")
	withEnv(t, "LATEXMK_ARGS_FILE", argsFile)
	runInDir(t, project, func() {
		var out, stderr bytes.Buffer
		a := app{stdin: strings.NewReader(""), stdout: &out, stderr: &stderr}
		if err := a.run([]string{"build"}); err != nil {
			t.Fatalf("build: %v\nstdout=%s\nstderr=%s", err, out.String(), stderr.String())
		}
	})

	assertFile(t, filepath.Join(project, "assets", "pdf", "chart.pdf"))
	args := readFile(t, argsFile)
	for _, want := range []string{"-cd", "-xelatex", "-output-directory=" + filepath.Join(project, "build"), filepath.Join(project, "main.tex")} {
		if !strings.Contains(args, want) {
			t.Fatalf("latexmk args missing %q:\n%s", want, args)
		}
	}
}

func TestDefaultCommandBuildsInTexProject(t *testing.T) {
	project := realPath(t, t.TempDir())
	fakeBin := t.TempDir()
	argsFile := filepath.Join(project, "latexmk.args")
	installFakeTool(t, fakeBin, "latexmk")
	writeFile(t, filepath.Join(project, "main.tex"), `\documentclass{article}\begin{document}x\end{document}`)

	withEnv(t, "PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	withEnv(t, "TEXGO_FAKE_TOOL", "1")
	withEnv(t, "LATEXMK_ARGS_FILE", argsFile)
	runInDir(t, project, func() {
		var out, stderr bytes.Buffer
		a := app{stdin: strings.NewReader(""), stdout: &out, stderr: &stderr}
		if err := a.run(nil); err != nil {
			t.Fatalf("default build: %v\nstdout=%s\nstderr=%s", err, out.String(), stderr.String())
		}
	})

	args := readFile(t, argsFile)
	if !strings.Contains(args, filepath.Join(project, "main.tex")) {
		t.Fatalf("default command did not build main.tex:\n%s", args)
	}
}

func TestBuildGuidesSetupWhenMainTexIsAmbiguous(t *testing.T) {
	project := realPath(t, t.TempDir())
	writeFile(t, filepath.Join(project, "a.tex"), `\documentclass{article}\begin{document}a\end{document}`)
	writeFile(t, filepath.Join(project, "b.tex"), `\documentclass{article}\begin{document}b\end{document}`)

	runInDir(t, project, func() {
		var out, stderr bytes.Buffer
		a := app{stdin: strings.NewReader(""), stdout: &out, stderr: &stderr}
		err := a.run([]string{"build"})
		if err == nil {
			t.Fatal("expected ambiguous build to fail")
		}
		if !strings.Contains(err.Error(), "texgo setup") {
			t.Fatalf("expected setup guidance, got: %v", err)
		}
	})
}

func TestSetupWritesConfigAndBuildUsesIt(t *testing.T) {
	project := realPath(t, t.TempDir())
	fakeBin := t.TempDir()
	argsFile := filepath.Join(project, "latexmk.args")
	mkdirAll(t, filepath.Join(project, "figures"))
	writeFile(t, filepath.Join(project, "main.tex"), `\documentclass{article}\begin{document}main\end{document}`)
	writeFile(t, filepath.Join(project, "paper.tex"), `\documentclass{article}\usepackage{graphicx}\begin{document}\includegraphics{figures/plot.png}\end{document}`)
	writeFile(t, filepath.Join(project, "figures", "plot.png"), "png")
	installFakeTool(t, fakeBin, "gm")
	installFakeTool(t, fakeBin, "latexmk")

	runInDir(t, project, func() {
		var out, stderr bytes.Buffer
		a := app{stdin: strings.NewReader("2\nlualatex\nout\nfigures\n"), stdout: &out, stderr: &stderr}
		if err := a.run([]string{"setup"}); err != nil {
			t.Fatalf("setup: %v\nstdout=%s\nstderr=%s", err, out.String(), stderr.String())
		}
	})

	configText := readFile(t, filepath.Join(project, configFileName))
	for _, want := range []string{"tex_file=paper.tex", "engine=lualatex", "build_dir=out", "figures_dir=figures"} {
		if !strings.Contains(configText, want) {
			t.Fatalf("config missing %q:\n%s", want, configText)
		}
	}

	withEnv(t, "PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	withEnv(t, "TEXGO_FAKE_TOOL", "1")
	withEnv(t, "LATEXMK_ARGS_FILE", argsFile)
	runInDir(t, project, func() {
		var out, stderr bytes.Buffer
		a := app{stdin: strings.NewReader(""), stdout: &out, stderr: &stderr}
		if err := a.run([]string{"build"}); err != nil {
			t.Fatalf("configured build: %v\nstdout=%s\nstderr=%s", err, out.String(), stderr.String())
		}
	})

	assertFile(t, filepath.Join(project, "figures", "pdf", "plot.pdf"))
	args := readFile(t, argsFile)
	for _, want := range []string{"-lualatex", "-output-directory=" + filepath.Join(project, "out"), filepath.Join(project, "paper.tex")} {
		if !strings.Contains(args, want) {
			t.Fatalf("configured build args missing %q:\n%s", want, args)
		}
	}
	if strings.Contains(args, filepath.Join(project, "main.tex")) {
		t.Fatalf("configured build unexpectedly used main.tex:\n%s", args)
	}
}

func TestSetupLetsUserChooseScannedImageDirectory(t *testing.T) {
	project := realPath(t, t.TempDir())
	writeFile(t, filepath.Join(project, "main.tex"), `\documentclass{article}\begin{document}main\end{document}`)
	writeFile(t, filepath.Join(project, "data", "plots", "chart.png"), "png")
	writeFile(t, filepath.Join(project, "notes", "readme.txt"), "not an image")

	runInDir(t, project, func() {
		var out, stderr bytes.Buffer
		a := app{stdin: strings.NewReader("\nxelatex\nbuild\n1\n"), stdout: &out, stderr: &stderr}
		if err := a.run([]string{"setup"}); err != nil {
			t.Fatalf("setup: %v\nstdout=%s\nstderr=%s", err, out.String(), stderr.String())
		}
		if !strings.Contains(stderr.String(), "Image directories found:") || !strings.Contains(stderr.String(), "data/plots") {
			t.Fatalf("setup did not show scanned image directories:\n%s", stderr.String())
		}
	})

	configText := readFile(t, filepath.Join(project, configFileName))
	if !strings.Contains(configText, "figures_dir=data/plots") {
		t.Fatalf("config did not save selected scanned image directory:\n%s", configText)
	}
}

func TestCleanRemovesBuildArtifactsAndCachedPDFs(t *testing.T) {
	project := realPath(t, t.TempDir())
	mkdirAll(t, filepath.Join(project, "build"))
	mkdirAll(t, filepath.Join(project, "figures", "pdf"))
	writeFile(t, filepath.Join(project, "build", "manuscript.pdf"), "pdf")
	writeFile(t, filepath.Join(project, "figures", "pdf", "logo.pdf"), "pdf")

	runInDir(t, project, func() {
		var out, stderr bytes.Buffer
		a := app{stdin: strings.NewReader(""), stdout: &out, stderr: &stderr}
		if err := a.run([]string{"clean", "--figures"}); err != nil {
			t.Fatalf("clean: %v\nstdout=%s\nstderr=%s", err, out.String(), stderr.String())
		}
	})

	assertNotFile(t, filepath.Join(project, "build", "manuscript.pdf"))
	assertNotFile(t, filepath.Join(project, "figures", "pdf", "logo.pdf"))
}

func installFakeTool(t *testing.T, dir, name string) {
	t.Helper()
	mkdirAll(t, dir)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, name)
	if err := os.Symlink(exe, target); err == nil {
		return
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, data, 0755); err != nil {
		t.Fatal(err)
	}
}

func runInDir(t *testing.T, dir string, fn func()) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatal(err)
		}
	}()
	fn()
}

func withEnv(t *testing.T, key, value string) {
	t.Helper()
	previous, ok := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(key, previous)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func realPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	mkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func assertFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		t.Fatalf("expected file to exist: %s", path)
	}
}

func assertNotFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		t.Fatalf("expected file to be absent: %s", path)
	}
}

func assertNoChildDirNamed(t *testing.T, parent, name string) {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() == name {
			t.Fatalf("expected %s not to contain child directory named %s", parent, name)
		}
	}
}
