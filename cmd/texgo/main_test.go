package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
			if logPath := os.Getenv("GM_START_LOG"); logPath != "" {
				log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
				if err != nil {
					return 1
				}
				if _, err := log.WriteString(filepath.Base(os.Args[2]) + "\n"); err != nil {
					_ = log.Close()
					return 1
				}
				if err := log.Close(); err != nil {
					return 1
				}
			}
			if releasePath := os.Getenv("GM_RELEASE_FILE"); releasePath != "" {
				for !fileExists(releasePath) {
					time.Sleep(5 * time.Millisecond)
				}
			}
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
		if forbidden := os.Getenv("LATEXMK_FORBIDDEN_FILE"); forbidden != "" && fileExists(forbidden) {
			return 3
		}
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
	for _, want := range []string{"Usage: texgo", "setup", "build", "images", "standalone article starter project"} {
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

func TestImagesWorkersLimitsConcurrentConversions(t *testing.T) {
	project := realPath(t, t.TempDir())
	fakeBin := t.TempDir()
	startLog := filepath.Join(project, "gm-started.log")
	releaseFile := filepath.Join(project, "release-gm")
	installFakeTool(t, fakeBin, "gm")
	for _, name := range []string{"one.png", "two.png", "three.png", "four.png"} {
		writeFile(t, filepath.Join(project, "figures", name), name)
	}

	withEnv(t, "PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	withEnv(t, "TEXGO_FAKE_TOOL", "1")
	withEnv(t, "GM_START_LOG", startLog)
	withEnv(t, "GM_RELEASE_FILE", releaseFile)

	var out, stderr bytes.Buffer
	done := make(chan error, 1)
	runInDir(t, project, func() {
		a := app{stdin: strings.NewReader(""), stdout: &out, stderr: &stderr}
		go func() {
			done <- a.run([]string{"images", "--workers", "2"})
		}()

		waitForLineCount(t, startLog, 2, releaseFile)
		time.Sleep(50 * time.Millisecond)
		if got := lineCount(startLog); got != 2 {
			writeFile(t, releaseFile, "go")
			<-done
			t.Fatalf("expected no more than 2 active conversions, got %d", got)
		}
		writeFile(t, releaseFile, "go")
		if err := <-done; err != nil {
			t.Fatalf("images: %v\nstdout=%s\nstderr=%s", err, out.String(), stderr.String())
		}
	})

	if got := lineCount(startLog); got != 4 {
		t.Fatalf("expected all 4 conversions to run, got %d", got)
	}
	for _, name := range []string{"one.pdf", "two.pdf", "three.pdf", "four.pdf"} {
		assertFile(t, filepath.Join(project, "figures", "pdf", name))
	}
}

func TestImagesConvertsOneSourceWhenExtensionsSharePDFTarget(t *testing.T) {
	project := realPath(t, t.TempDir())
	fakeBin := t.TempDir()
	startLog := filepath.Join(project, "gm-started.log")
	installFakeTool(t, fakeBin, "gm")
	writeFile(t, filepath.Join(project, "figures", "plot.png"), "png")
	writeFile(t, filepath.Join(project, "figures", "plot.jpg"), "jpg")

	withEnv(t, "PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	withEnv(t, "TEXGO_FAKE_TOOL", "1")
	withEnv(t, "GM_START_LOG", startLog)
	runInDir(t, project, func() {
		var out, stderr bytes.Buffer
		a := app{stdin: strings.NewReader(""), stdout: &out, stderr: &stderr}
		if err := a.run([]string{"images", "--workers", "2"}); err != nil {
			t.Fatalf("images: %v\nstdout=%s\nstderr=%s", err, out.String(), stderr.String())
		}
	})

	if got := lineCount(startLog); got != 1 {
		t.Fatalf("expected one conversion targeting plot.pdf, got %d", got)
	}
	if got := readFile(t, filepath.Join(project, "figures", "pdf", "plot.pdf")); got != "jpg" {
		t.Fatalf("expected the existing extension order to retain jpg source, got %q", got)
	}
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

func TestBuildIsolatesAndRestoresConflictingSourceDirectoryArtifacts(t *testing.T) {
	project := realPath(t, t.TempDir())
	fakeBin := t.TempDir()
	argsFile := filepath.Join(project, "latexmk.args")
	installFakeTool(t, fakeBin, "latexmk")
	writeFile(t, filepath.Join(project, "manuscript.tex"), `\documentclass{article}\begin{document}x\end{document}`)
	for _, ext := range []string{".aux", ".bbl", ".out", ".fdb_latexmk"} {
		writeFile(t, filepath.Join(project, "manuscript"+ext), "stale")
	}
	writeFile(t, filepath.Join(project, "build", "manuscript.fdb_latexmk"), "failed build state")
	writeFile(t, filepath.Join(project, "manuscript.pdf"), "keep")

	withEnv(t, "PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	withEnv(t, "TEXGO_FAKE_TOOL", "1")
	withEnv(t, "LATEXMK_ARGS_FILE", argsFile)
	withEnv(t, "LATEXMK_FORBIDDEN_FILE", filepath.Join(project, "manuscript.bbl"))
	runInDir(t, project, func() {
		var out, stderr bytes.Buffer
		a := app{stdin: strings.NewReader(""), stdout: &out, stderr: &stderr}
		if err := a.run([]string{"build", "--no-images"}); err != nil {
			t.Fatalf("build: %v\nstdout=%s\nstderr=%s", err, out.String(), stderr.String())
		}
	})

	for _, ext := range []string{".aux", ".bbl", ".out", ".fdb_latexmk"} {
		path := filepath.Join(project, "manuscript"+ext)
		assertFile(t, path)
		if got := readFile(t, path); got != "stale" {
			t.Fatalf("expected restored artifact content at %s, got %q", path, got)
		}
	}
	assertNotFile(t, filepath.Join(project, "build", "manuscript.fdb_latexmk"))
	assertFile(t, filepath.Join(project, "manuscript.pdf"))
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

func TestInitCreatesStandaloneJournalArticleTemplate(t *testing.T) {
	project := realPath(t, t.TempDir())
	target := filepath.Join(project, "paper")

	runInDir(t, project, func() {
		var out, stderr bytes.Buffer
		a := app{stdin: strings.NewReader(""), stdout: &out, stderr: &stderr}
		if err := a.run([]string{"init", target}); err != nil {
			t.Fatalf("init: %v\nstdout=%s\nstderr=%s", err, out.String(), stderr.String())
		}
	})

	manuscript := readFile(t, filepath.Join(target, "template", "manuscript.tex"))
	for _, want := range []string{
		`\documentclass[`,
		`]{article}`,
		`\addbibresource{bibliography/references.bib}`,
		`\begin{abstract}`,
		`\section{Introduction}`,
		`\includegraphics[width=0.95\linewidth]{figures/pdf/logo.pdf}`,
		`\printbibliography`,
		`mailto:i@yaho7.cn`,
		`https://github.com/Yaho7/texgo`,
		`https://yaho7.cn`,
	} {
		if !strings.Contains(manuscript, want) {
			t.Fatalf("generated manuscript missing %q:\n%s", want, manuscript)
		}
	}
	if strings.Contains(manuscript, "styles/") {
		t.Fatalf("generated manuscript must not depend on styles/:\n%s", manuscript)
	}
	references := readFile(t, filepath.Join(target, "template", "bibliography", "references.bib"))
	if !strings.Contains(references, "@article{seemann_droplet_2012") {
		t.Fatalf("generated bibliography missing sample citation:\n%s", references)
	}
	if !dirExists(filepath.Join(target, "template", "figures")) {
		t.Fatalf("generated template missing figures directory")
	}
	logo, err := os.ReadFile(filepath.Join(target, "template", "figures", "logo.png"))
	if err != nil {
		t.Fatalf("generated template missing embedded logo: %v", err)
	}
	if len(logo) < 8 || string(logo[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Fatalf("generated logo is not a PNG asset")
	}
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

func waitForLineCount(t *testing.T, path string, want int, releaseFile string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if lineCount(path) >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	writeFile(t, releaseFile, "go")
	t.Fatalf("timed out waiting for %d conversions to start; got %d", want, lineCount(path))
}

func lineCount(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return len(strings.Fields(string(data)))
}
