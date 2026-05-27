package main

import (
	"bufio"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	configFileName      = ".texgo.conf"
	defaultImageWorkers = 4
)

var imageExts = []string{"png", "jpg", "jpeg", "gif", "tif", "tiff", "bmp", "svg"}

var latexTransientExts = []string{
	".aux", ".bbl", ".bcf", ".blg", ".fdb_latexmk", ".fls", ".log", ".out",
	".run.xml", ".synctex.gz", ".toc", ".lof", ".lot", ".nav", ".snm", ".vrb",
	".xdv", ".dvi",
}

//go:embed template-assets/logo.png
var templateLogoPNG []byte

type app struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

type config struct {
	TexFile       string
	FiguresDir    string
	PDFDir        string
	BuildDir      string
	Engine        string
	ConvertImages string
}

type imageOptions struct {
	ProjectDir string
	FiguresDir string
	PDFDir     string
	TexFile    string
	Workers    int
	Force      bool
	Prune      bool
}

type buildOptions struct {
	ProjectDir    string
	FiguresDir    string
	PDFDir        string
	BuildDir      string
	Engine        string
	ConvertImages bool
	OpenPDF       bool
	TexFile       string
	Workers       int
}

type imageConversion struct {
	source string
	target string
	name   string
}

type synchronizedWriter struct {
	mu  *sync.Mutex
	dst io.Writer
}

func (w synchronizedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.dst.Write(data)
}

func main() {
	a := app{stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr}
	if err := a.run(os.Args[1:]); err != nil {
		fmt.Fprintf(a.stderr, "texgo: %v\n", err)
		os.Exit(1)
	}
}

func (a app) run(args []string) error {
	command := "help"
	if len(args) == 0 {
		if fileExists(configFileName) || hasTexFiles(".") {
			command = "build"
		}
	} else {
		command = args[0]
		args = args[1:]
	}

	switch command {
	case "setup":
		return a.runSetup(args)
	case "build":
		return a.runBuild(args)
	case "images":
		return a.runImages(args)
	case "clean":
		return a.runClean(args)
	case "init":
		return a.runInit(args)
	case "doctor":
		return a.runDoctor()
	case "help", "-h", "--help":
		a.printHelp()
		return nil
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

func (a app) printHelp() {
	fmt.Fprint(a.stdout, `Usage: texgo <command> [options]

Commands:
  setup                  Create or update workspace configuration interactively.
  build [tex-file]       Convert figures, then compile with latexmk.
  images                 Convert image files to cached PDFs.
  clean [--figures]      Remove build artifacts, optionally cached figure PDFs.
  init [directory]       Create a standalone article starter project.
  doctor                 Check required external commands.
  help                   Show this help.

Common options:
  --project-dir DIR      Project root. Defaults to the current directory.
  --figures-dir DIR      Figure source directory. Defaults to auto-detection.
  --pdf-dir DIR          Cached PDF directory. Defaults to <figures-dir>/pdf.
  --build-dir DIR        Build output directory. Defaults to build.
  --tex-file FILE        Main TeX file for image auto-detection.
  --workers N            Maximum concurrent image conversions. Default: 4.

Build options:
  --engine ENGINE        latexmk engine: xelatex, pdflatex, lualatex. Default: xelatex.
  --no-images            Skip image conversion before compiling.
  --open                 Open the generated PDF after a successful build.

Image options:
  --force                Rebuild cached PDFs even when they look up to date.
  --no-prune             Do not delete cached PDFs with no matching source image.
`)
}

func extractProjectDir(args []string) (string, error) {
	projectDir := "."
	for i := 0; i < len(args); i++ {
		if args[i] == "--project-dir" {
			if i+1 >= len(args) {
				return "", errors.New("--project-dir requires a value")
			}
			projectDir = args[i+1]
			i++
		}
	}
	return filepath.Abs(projectDir)
}

func parseWorkers(value string) (int, error) {
	workers, err := strconv.Atoi(value)
	if err != nil || workers < 1 {
		return 0, errors.New("--workers requires a positive integer")
	}
	return workers, nil
}

func loadConfig(projectDir string) (config, error) {
	var cfg config
	data, err := os.ReadFile(filepath.Join(projectDir, configFileName))
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "tex_file":
			cfg.TexFile = parts[1]
		case "figures_dir":
			cfg.FiguresDir = parts[1]
		case "pdf_dir":
			cfg.PDFDir = parts[1]
		case "build_dir":
			cfg.BuildDir = parts[1]
		case "engine":
			cfg.Engine = parts[1]
		case "convert_images":
			cfg.ConvertImages = parts[1]
		}
	}
	return cfg, nil
}

func writeConfig(projectDir string, cfg config) error {
	content := fmt.Sprintf(`# texgo workspace configuration
tex_file=%s
engine=%s
build_dir=%s
figures_dir=%s
convert_images=%s
`, cfg.TexFile, cfg.Engine, cfg.BuildDir, cfg.FiguresDir, cfg.ConvertImages)
	return os.WriteFile(filepath.Join(projectDir, configFileName), []byte(content), 0644)
}

func resolveInProject(projectDir, path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(projectDir, path)
}

func relativeToProject(projectDir, path string) string {
	rel, err := filepath.Rel(projectDir, path)
	if err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
}

func hasTexFiles(root string) bool {
	files, _ := findTexFiles(root)
	return len(files) > 0
}

func findTexFiles(projectDir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == projectDir {
			return nil
		}
		rel, err := filepath.Rel(projectDir, path)
		if err != nil {
			return err
		}
		depth := strings.Count(rel, string(os.PathSeparator)) + 1
		if d.IsDir() && depth > 3 {
			return filepath.SkipDir
		}
		if !d.IsDir() && depth <= 3 && strings.EqualFold(filepath.Ext(path), ".tex") {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func defaultTexFile(projectDir string) (string, error) {
	for _, candidate := range []string{"main.tex", "manuscript.tex", "paper.tex", "thesis.tex"} {
		path := filepath.Join(projectDir, candidate)
		if fileExists(path) {
			return path, nil
		}
	}
	files, err := findTexFiles(projectDir)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", errors.New("no .tex file found; pass one explicitly")
	}
	if len(files) > 1 {
		return "", errors.New("multiple .tex files found; run 'texgo setup' to save the main file, or pass it explicitly with 'texgo build main.tex'")
	}
	return files[0], nil
}

func (a app) runImages(args []string) error {
	projectDir, err := extractProjectDir(args)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(projectDir)
	if err != nil {
		return err
	}
	opts := imageOptions{
		ProjectDir: projectDir,
		FiguresDir: cfg.FiguresDir,
		PDFDir:     cfg.PDFDir,
		TexFile:    cfg.TexFile,
		Workers:    defaultImageWorkers,
		Prune:      true,
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project-dir":
			i++
		case "--figures-dir":
			if i+1 >= len(args) {
				return errors.New("--figures-dir requires a value")
			}
			opts.FiguresDir = args[i+1]
			i++
		case "--pdf-dir":
			if i+1 >= len(args) {
				return errors.New("--pdf-dir requires a value")
			}
			opts.PDFDir = args[i+1]
			i++
		case "--tex-file":
			if i+1 >= len(args) {
				return errors.New("--tex-file requires a value")
			}
			opts.TexFile = args[i+1]
			i++
		case "--workers":
			if i+1 >= len(args) {
				return errors.New("--workers requires a value")
			}
			opts.Workers, err = parseWorkers(args[i+1])
			if err != nil {
				return err
			}
			i++
		case "--force":
			opts.Force = true
		case "--no-prune":
			opts.Prune = false
		case "-h", "--help":
			a.printHelp()
			return nil
		default:
			return fmt.Errorf("unknown images option: %s", args[i])
		}
	}
	return a.convertImages(opts)
}

func (a app) convertImages(opts imageOptions) error {
	figureDirs, err := resolveFigureDirs(opts.ProjectDir, opts.FiguresDir, opts.TexFile)
	if err != nil {
		return err
	}
	if len(figureDirs) == 0 {
		fmt.Fprintln(a.stdout, "No figure directories found; skipping image conversion")
		return nil
	}
	if _, err := exec.LookPath("gm"); err != nil {
		return errors.New("GraphicsMagick is not installed. Install it first, then rerun this command")
	}
	if opts.PDFDir != "" && len(figureDirs) > 1 {
		return errors.New("--pdf-dir can only be used with a single --figures-dir")
	}
	for _, dir := range figureDirs {
		targetPDFDir := defaultPDFDir(dir)
		if opts.PDFDir != "" {
			targetPDFDir = resolveInProject(opts.ProjectDir, opts.PDFDir)
		}
		if err := a.convertImagesDir(dir, targetPDFDir, opts.Workers, opts.Force, opts.Prune); err != nil {
			return err
		}
	}
	return nil
}

func resolveFigureDirs(projectDir, figuresDir, texFile string) ([]string, error) {
	if figuresDir != "" {
		return uniqueExistingDirs([]string{resolveInProject(projectDir, figuresDir)}), nil
	}
	var dirs []string
	if texFile != "" {
		texPath := resolveInProject(projectDir, texFile)
		discovered, err := discoverGraphicsDirs(texPath)
		if err != nil {
			return nil, err
		}
		dirs = discovered
	}
	if len(dirs) == 0 {
		scanned, err := findImageDirs(projectDir)
		if err != nil {
			return nil, err
		}
		dirs = scanned
		if len(dirs) == 0 {
			dirs = commonGraphicsDirs(projectDir)
			if texFile != "" {
				texDir := filepath.Dir(resolveInProject(projectDir, texFile))
				if texDir != projectDir {
					dirs = append(dirs, commonGraphicsDirs(texDir)...)
				}
			}
		}
	}
	return uniqueExistingDirs(dirs), nil
}

func uniqueExistingDirs(dirs []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, dir := range dirs {
		if dir == "" || seen[dir] || !dirExists(dir) {
			continue
		}
		seen[dir] = true
		result = append(result, dir)
	}
	sort.Strings(result)
	return result
}

func commonGraphicsDirs(projectDir string) []string {
	var dirs []string
	for _, dir := range []string{"figures", "images", "img", "assets"} {
		if path := existingChildDir(projectDir, dir); path != "" {
			dirs = append(dirs, path)
		}
	}
	return dirs
}

func findImageDirs(projectDir string) ([]string, error) {
	var dirs []string
	err := filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != projectDir && shouldSkipImageScanDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if isSupportedImagePath(path) {
			dirs = append(dirs, filepath.Dir(path))
		}
		return nil
	})
	return uniqueExistingDirs(dirs), err
}

func shouldSkipImageScanDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch strings.ToLower(name) {
	case "build", "dist", "out", "pdf", "node_modules":
		return true
	default:
		return false
	}
}

func existingChildDir(parent, name string) string {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.EqualFold(entry.Name(), name) {
			return filepath.Join(parent, entry.Name())
		}
	}
	return ""
}

var includeGraphicsRE = regexp.MustCompile(`\\includegraphics(\[[^\]]*\])?\{([^}]+)\}`)

func extractIncludeGraphicsRefs(texFile string) ([]string, error) {
	data, err := os.ReadFile(texFile)
	if err != nil {
		return nil, err
	}
	var refs []string
	for _, line := range strings.Split(string(data), "\n") {
		if idx := strings.Index(line, "%"); idx >= 0 {
			line = line[:idx]
		}
		for _, match := range includeGraphicsRE.FindAllStringSubmatch(line, -1) {
			refs = append(refs, strings.TrimSpace(match[2]))
		}
	}
	return refs, nil
}

func discoverGraphicsDirs(texFile string) ([]string, error) {
	refs, err := extractIncludeGraphicsRefs(texFile)
	if err != nil {
		return nil, err
	}
	texDir := filepath.Dir(texFile)
	var dirs []string
	for _, ref := range refs {
		if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") || ref == "" {
			continue
		}
		resolved := resolveInProject(texDir, ref)
		dir := filepath.Dir(resolved)
		filename := filepath.Base(resolved)
		name := strings.TrimSuffix(filename, filepath.Ext(filename))
		if strings.EqualFold(filepath.Base(dir), "pdf") && dirExists(filepath.Dir(dir)) && matchingSourceImage(filepath.Dir(dir), name) != "" {
			dirs = append(dirs, filepath.Dir(dir))
			continue
		}
		if dirExists(dir) && isSupportedImagePath(filename) {
			dirs = append(dirs, dir)
			continue
		}
		if dirExists(dir) && matchingSourceImage(dir, filename) != "" {
			dirs = append(dirs, dir)
		}
	}
	return uniqueExistingDirs(dirs), nil
}

func isSupportedImagePath(path string) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	for _, allowed := range imageExts {
		if ext == allowed {
			return true
		}
	}
	return false
}

func matchingSourceImage(dir, name string) string {
	for _, ext := range imageExts {
		matches, _ := filepath.Glob(filepath.Join(dir, "*."+ext))
		matchesUpper, _ := filepath.Glob(filepath.Join(dir, "*."+strings.ToUpper(ext)))
		for _, match := range append(matches, matchesUpper...) {
			base := filepath.Base(match)
			if strings.EqualFold(strings.TrimSuffix(base, filepath.Ext(base)), name) || strings.EqualFold(base, name) {
				return match
			}
		}
	}
	return ""
}

func defaultPDFDir(figuresDir string) string {
	if existing := existingChildDir(figuresDir, "pdf"); existing != "" {
		return existing
	}
	return filepath.Join(figuresDir, "pdf")
}

func (a app) convertImagesDir(figuresDir, pdfDir string, workers int, force, prune bool) error {
	if !dirExists(figuresDir) {
		return fmt.Errorf("figures directory not found: %s", figuresDir)
	}
	if err := os.MkdirAll(pdfDir, 0755); err != nil {
		return err
	}
	var pending []imageConversion
	pendingTarget := make(map[string]int)
	for _, ext := range imageExts {
		matches, _ := filepath.Glob(filepath.Join(figuresDir, "*."+ext))
		matchesUpper, _ := filepath.Glob(filepath.Join(figuresDir, "*."+strings.ToUpper(ext)))
		for _, img := range append(matches, matchesUpper...) {
			name := strings.TrimSuffix(filepath.Base(img), filepath.Ext(img))
			pdf := filepath.Join(pdfDir, name+".pdf")
			needsConvert := force || !fileExists(pdf) || newerThan(img, pdf)
			if needsConvert {
				conversion := imageConversion{source: img, target: pdf, name: name}
				if index, exists := pendingTarget[pdf]; exists {
					pending[index] = conversion
				} else {
					pendingTarget[pdf] = len(pending)
					pending = append(pending, conversion)
				}
			}
		}
	}
	if err := a.runImageConversions(pending, workers); err != nil {
		return err
	}
	if prune {
		pdfs, _ := filepath.Glob(filepath.Join(pdfDir, "*.pdf"))
		for _, pdf := range pdfs {
			name := strings.TrimSuffix(filepath.Base(pdf), filepath.Ext(pdf))
			if matchingSourceImage(figuresDir, name) == "" {
				fmt.Fprintf(a.stdout, "Removing stale cached PDF: %s.pdf\n", name)
				if err := os.Remove(pdf); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (a app) runImageConversions(pending []imageConversion, workers int) error {
	if len(pending) == 0 {
		return nil
	}
	if workers > len(pending) {
		workers = len(pending)
	}

	var outputMu sync.Mutex
	stdout := synchronizedWriter{mu: &outputMu, dst: a.stdout}
	stderr := synchronizedWriter{mu: &outputMu, dst: a.stderr}
	jobs := make(chan imageConversion)
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				cmd := exec.Command("gm", "convert", job.source, job.target)
				cmd.Stdout = stdout
				cmd.Stderr = stderr
				if err := cmd.Run(); err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("convert %s: %w", filepath.Base(job.source), err)
					}
					errMu.Unlock()
				}
			}
		}()
	}
	for _, job := range pending {
		fmt.Fprintf(stdout, "Converting %s -> %s.pdf\n", filepath.Base(job.source), job.name)
		jobs <- job
	}
	close(jobs)
	wg.Wait()
	return firstErr
}

func (a app) runBuild(args []string) error {
	projectDir, err := extractProjectDir(args)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(projectDir)
	if err != nil {
		return err
	}
	opts := buildOptions{
		ProjectDir:    projectDir,
		FiguresDir:    cfg.FiguresDir,
		PDFDir:        cfg.PDFDir,
		BuildDir:      defaultString(cfg.BuildDir, "build"),
		Engine:        defaultString(cfg.Engine, "xelatex"),
		ConvertImages: cfg.ConvertImages != "0",
		TexFile:       cfg.TexFile,
		Workers:       defaultImageWorkers,
	}
	positionalTex := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project-dir":
			i++
		case "--figures-dir":
			if i+1 >= len(args) {
				return errors.New("--figures-dir requires a value")
			}
			opts.FiguresDir = args[i+1]
			i++
		case "--pdf-dir":
			if i+1 >= len(args) {
				return errors.New("--pdf-dir requires a value")
			}
			opts.PDFDir = args[i+1]
			i++
		case "--build-dir":
			if i+1 >= len(args) {
				return errors.New("--build-dir requires a value")
			}
			opts.BuildDir = args[i+1]
			i++
		case "--engine":
			if i+1 >= len(args) {
				return errors.New("--engine requires a value")
			}
			opts.Engine = args[i+1]
			i++
		case "--workers":
			if i+1 >= len(args) {
				return errors.New("--workers requires a value")
			}
			opts.Workers, err = parseWorkers(args[i+1])
			if err != nil {
				return err
			}
			i++
		case "--no-images":
			opts.ConvertImages = false
		case "--open":
			opts.OpenPDF = true
		case "-h", "--help":
			a.printHelp()
			return nil
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("unknown build option: %s", args[i])
			}
			if positionalTex {
				return errors.New("only one tex file can be provided")
			}
			opts.TexFile = args[i]
			positionalTex = true
		}
	}
	return a.build(opts)
}

func (a app) build(opts buildOptions) error {
	switch opts.Engine {
	case "xelatex", "pdflatex", "lualatex":
	default:
		return fmt.Errorf("unsupported engine: %s", opts.Engine)
	}
	buildDir := resolveInProject(opts.ProjectDir, opts.BuildDir)
	texFile := resolveInProject(opts.ProjectDir, opts.TexFile)
	if opts.TexFile == "" {
		var err error
		texFile, err = defaultTexFile(opts.ProjectDir)
		if err != nil {
			return err
		}
	}
	if !fileExists(texFile) {
		return fmt.Errorf("tex file not found: %s", texFile)
	}
	if _, err := exec.LookPath("latexmk"); err != nil {
		return errors.New("latexmk is not installed or not on PATH")
	}
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return err
	}
	if opts.ConvertImages {
		if err := a.convertImages(imageOptions{
			ProjectDir: opts.ProjectDir,
			FiguresDir: opts.FiguresDir,
			PDFDir:     opts.PDFDir,
			TexFile:    texFile,
			Workers:    opts.Workers,
			Prune:      true,
		}); err != nil {
			return err
		}
	}
	latexArgs := []string{
		"-cd",
		"-" + opts.Engine,
		"-synctex=1",
		"-interaction=nonstopmode",
		"-file-line-error",
		"-output-directory=" + buildDir,
		texFile,
	}
	restoreArtifacts, err := isolateSourceArtifacts(texFile, buildDir)
	if err != nil {
		return err
	}
	cmd := exec.Command("latexmk", latexArgs...)
	cmd.Stdout = a.stdout
	cmd.Stderr = a.stderr
	runErr := cmd.Run()
	restoreErr := restoreArtifacts()
	if runErr != nil {
		if restoreErr != nil {
			return fmt.Errorf("%w; additionally could not restore source artifacts: %v", runErr, restoreErr)
		}
		return runErr
	}
	if restoreErr != nil {
		return restoreErr
	}
	if opts.OpenPDF {
		pdfFile := filepath.Join(buildDir, strings.TrimSuffix(filepath.Base(texFile), filepath.Ext(texFile))+".pdf")
		if !fileExists(pdfFile) {
			return fmt.Errorf("build finished, but PDF was not found: %s", pdfFile)
		}
		return openFile(pdfFile)
	}
	return nil
}

func isolateSourceArtifacts(texFile, buildDir string) (func() error, error) {
	noop := func() error { return nil }
	texDir := filepath.Dir(texFile)
	if filepath.Clean(texDir) == filepath.Clean(buildDir) {
		return noop, nil
	}

	stem := strings.TrimSuffix(filepath.Base(texFile), filepath.Ext(texFile))
	var artifacts []string
	for _, ext := range latexTransientExts {
		path := filepath.Join(texDir, stem+ext)
		if fileExists(path) {
			artifacts = append(artifacts, path)
		}
	}
	if len(artifacts) == 0 {
		return noop, nil
	}

	latexmkDatabase := filepath.Join(buildDir, stem+".fdb_latexmk")
	if err := os.Remove(latexmkDatabase); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	backupDir, err := os.MkdirTemp(buildDir, ".texgo-source-artifacts-")
	if err != nil {
		return nil, err
	}
	for index, path := range artifacts {
		backupPath := filepath.Join(backupDir, filepath.Base(path))
		if err := os.Rename(path, backupPath); err != nil {
			for _, moved := range artifacts[:index] {
				_ = os.Rename(filepath.Join(backupDir, filepath.Base(moved)), moved)
			}
			_ = os.Remove(backupDir)
			return nil, err
		}
	}

	restored := false
	return func() error {
		if restored {
			return nil
		}
		restored = true
		var firstErr error
		for _, path := range artifacts {
			backupPath := filepath.Join(backupDir, filepath.Base(path))
			if err := os.Rename(backupPath, path); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		if err := os.Remove(backupDir); err != nil && firstErr == nil {
			firstErr = err
		}
		return firstErr
	}, nil
}

func (a app) runClean(args []string) error {
	projectDir, err := extractProjectDir(args)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(projectDir)
	if err != nil {
		return err
	}
	buildDir := defaultString(cfg.BuildDir, "build")
	figuresDir := cfg.FiguresDir
	cleanFigures := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project-dir":
			i++
		case "--figures-dir":
			if i+1 >= len(args) {
				return errors.New("--figures-dir requires a value")
			}
			figuresDir = args[i+1]
			i++
		case "--build-dir":
			if i+1 >= len(args) {
				return errors.New("--build-dir requires a value")
			}
			buildDir = args[i+1]
			i++
		case "--figures":
			cleanFigures = true
		case "-h", "--help":
			a.printHelp()
			return nil
		default:
			return fmt.Errorf("unknown clean option: %s", args[i])
		}
	}
	resolvedBuildDir := resolveInProject(projectDir, buildDir)
	if err := os.RemoveAll(resolvedBuildDir); err != nil {
		return err
	}
	fmt.Fprintf(a.stdout, "Removed build directory: %s\n", resolvedBuildDir)
	if cleanFigures {
		var dirs []string
		if figuresDir != "" {
			dirs = []string{resolveInProject(projectDir, figuresDir)}
		} else {
			dirs = commonGraphicsDirs(projectDir)
		}
		for _, dir := range uniqueExistingDirs(dirs) {
			pdfDir := filepath.Join(dir, "pdf")
			if err := os.RemoveAll(pdfDir); err != nil {
				return err
			}
			fmt.Fprintf(a.stdout, "Removed cached figure PDFs: %s\n", pdfDir)
		}
	}
	return nil
}

func (a app) runSetup(args []string) error {
	projectDir, err := extractProjectDir(args)
	if err != nil {
		return err
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project-dir":
			i++
		case "-h", "--help":
			a.printHelp()
			return nil
		default:
			return fmt.Errorf("unknown setup option: %s", args[i])
		}
	}
	cfg, err := loadConfig(projectDir)
	if err != nil {
		return err
	}
	texFiles, err := findTexFiles(projectDir)
	if err != nil {
		return err
	}
	if len(texFiles) == 0 {
		return fmt.Errorf("no .tex file found in %s", projectDir)
	}
	reader := bufio.NewReader(a.stdin)
	selectedTex := ""
	if len(texFiles) == 1 {
		def := relativeToProject(projectDir, texFiles[0])
		selectedTex = a.prompt(reader, "Main TeX file", defaultString(cfg.TexFile, def))
	} else {
		fmt.Fprintln(a.stderr, "Select the main TeX file:")
		for i, tex := range texFiles {
			fmt.Fprintf(a.stderr, "  %d) %s\n", i+1, relativeToProject(projectDir, tex))
		}
		choiceRaw := a.prompt(reader, "Choice", "1")
		choice, err := strconv.Atoi(choiceRaw)
		if err != nil || choice < 1 || choice > len(texFiles) {
			return fmt.Errorf("invalid choice: %s", choiceRaw)
		}
		selectedTex = relativeToProject(projectDir, texFiles[choice-1])
	}
	engine := a.prompt(reader, "Engine (xelatex, pdflatex, lualatex)", defaultString(cfg.Engine, "xelatex"))
	switch engine {
	case "xelatex", "pdflatex", "lualatex":
	default:
		return fmt.Errorf("unsupported engine: %s", engine)
	}
	buildDir := a.prompt(reader, "Build output directory", defaultString(cfg.BuildDir, "build"))
	detected, err := discoverGraphicsDirs(resolveInProject(projectDir, selectedTex))
	if err != nil {
		return err
	}
	scanned, err := findImageDirs(projectDir)
	if err != nil {
		return err
	}
	imageDirs := uniqueExistingDirs(append(detected, scanned...))
	if len(imageDirs) > 0 {
		fmt.Fprintln(a.stderr, "Image directories found:")
		for i, dir := range imageDirs {
			fmt.Fprintf(a.stderr, "  %d) %s\n", i+1, relativeToProject(projectDir, dir))
		}
		if len(detected) > 0 {
			fmt.Fprintln(a.stderr, "Matched includegraphics references:")
			for _, dir := range detected {
				fmt.Fprintf(a.stderr, "  - %s\n", relativeToProject(projectDir, dir))
			}
		}
	} else {
		fmt.Fprintln(a.stderr, "No image directories found.")
	}
	figDefault := defaultString(cfg.FiguresDir, "auto")
	figuresChoice := a.prompt(reader, "Figure directory (auto, number, explicit path, or - to disable image conversion)", figDefault)
	figuresDir := figuresChoice
	convertImages := "1"
	if choice, err := strconv.Atoi(figuresChoice); err == nil {
		if choice < 1 || choice > len(imageDirs) {
			return fmt.Errorf("invalid figure directory choice: %s", figuresChoice)
		}
		figuresDir = relativeToProject(projectDir, imageDirs[choice-1])
	} else {
		switch figuresChoice {
		case "auto", "":
			figuresDir = ""
		case "-":
			figuresDir = ""
			convertImages = "0"
		}
	}
	newCfg := config{
		TexFile:       selectedTex,
		Engine:        engine,
		BuildDir:      buildDir,
		FiguresDir:    figuresDir,
		ConvertImages: convertImages,
	}
	if err := writeConfig(projectDir, newCfg); err != nil {
		return err
	}
	fmt.Fprintf(a.stdout, "Saved configuration: %s\n", filepath.Join(projectDir, configFileName))
	return nil
}

func (a app) prompt(reader *bufio.Reader, label, def string) string {
	fmt.Fprintf(a.stderr, "%s [%s]: ", label, def)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return def
	}
	return answer
}

func (a app) runInit(args []string) error {
	targetDir := "."
	if len(args) > 1 {
		return errors.New("init accepts at most one directory")
	}
	if len(args) == 1 {
		targetDir = args[0]
	}
	targetAbs, err := filepath.Abs(targetDir)
	if err != nil {
		return err
	}
	if fileExists(filepath.Join(targetAbs, "template")) {
		return fmt.Errorf("target already contains template/: %s", targetAbs)
	}
	if err := os.MkdirAll(targetAbs, 0755); err != nil {
		return err
	}
	if err := createMinimalTemplate(filepath.Join(targetAbs, "template")); err != nil {
		return err
	}
	fmt.Fprintf(a.stdout, "Created LaTeX template in: %s\n", filepath.Join(targetAbs, "template"))
	return nil
}

func createMinimalTemplate(target string) error {
	if err := os.MkdirAll(filepath.Join(target, "figures"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(target, "bibliography"), 0755); err != nil {
		return err
	}
	content := `%----------------------------------------------------------------------------------------
% PACKAGES AND DOCUMENT CONFIGURATION
%----------------------------------------------------------------------------------------
% Disable PDF image compression when using converted PDF figures with XeLaTeX/dvipdfmx.
\special{dvipdfmx:config z 0}

\documentclass[
	letterpaper,
	10pt,
	fleqn
]{article}

\usepackage[margin=1in]{geometry}
\usepackage{amsmath}
\usepackage{adjustbox}
\usepackage{graphicx}
\usepackage{float}
\usepackage{fancyhdr}
\usepackage[hidelinks]{hyperref}
\usepackage[backend=biber,style=numeric]{biblatex}
\addbibresource{bibliography/references.bib}

\setcounter{secnumdepth}{0}
\setcounter{page}{1}

\pagestyle{fancy}
\fancyhf{}
\lhead{Paper Title}
\cfoot{\thepage}
\rfoot{\href{https://github.com/Yaho7/texgo}{github.com/Yaho7/texgo}}
\renewcommand{\headrulewidth}{0.4pt}
\renewcommand{\footrulewidth}{0.4pt}

\newcommand{\keywords}[1]{\par\medskip\noindent\textbf{Keywords:} #1}

%----------------------------------------------------------------------------------------
% TITLE SECTION
%----------------------------------------------------------------------------------------
\title{Paper Title}
\author{%
	Author Name\textsuperscript{1}\thanks{Corresponding author: \href{mailto:i@yaho7.cn}{i@yaho7.cn}\\
	Blog: \href{https://yaho7.cn}{yaho7.cn}\\
	Template generated with \href{https://github.com/Yaho7/texgo}{texgo}; replace this information before submission.}\\
	\small\textsuperscript{\textbf{1}}Department, University, City, Country
}
\date{}

\begin{document}
\maketitle

\begin{abstract}
	\noindent Write the abstract here.

	\keywords{keyword; keyword; keyword}
\end{abstract}

%----------------------------------------------------------------------------------------

\section{Introduction}
Start writing here and cite relevant work~\cite{seemann_droplet_2012}.

\begin{figure}[htbp]
	\centering
	\IfFileExists{figures/pdf/logo.pdf}{%
		\includegraphics[width=0.95\linewidth]{figures/pdf/logo.pdf}%
	}{%
		\fbox{\parbox[c][4cm][c]{0.85\linewidth}{\centering Add a source image under \texttt{figures/} and run \texttt{texgo images}.}}%
	}
	\caption{Example figure.}
	\label{fig:example}
\end{figure}

\section{Conclusions}
Summarize the conclusions here.

%----------------------------------------------------------------------------------------
% REFERENCES
%----------------------------------------------------------------------------------------
\newpage
\printbibliography

\end{document}
`
	references := `@article{seemann_droplet_2012,
	title = {Replace with the title of your reference},
	author = {Author, First and Author, Second},
	journal = {Journal Name},
	year = {2012},
	volume = {1},
	pages = {1--10}
}
`
	if err := os.WriteFile(filepath.Join(target, "manuscript.tex"), []byte(content), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(target, "bibliography", "references.bib"), []byte(references), 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(target, "figures", "logo.png"), templateLogoPNG, 0644)
}

func (a app) runDoctor() error {
	missing := false
	for _, name := range []string{"gm", "latexmk"} {
		path, err := exec.LookPath(name)
		if err != nil {
			fmt.Fprintf(a.stdout, "Missing: %s\n", name)
			missing = true
		} else {
			fmt.Fprintf(a.stdout, "OK: %s (%s)\n", name, path)
		}
	}
	if missing {
		return errors.New("missing required commands")
	}
	return nil
}

func openFile(path string) error {
	var cmd *exec.Cmd
	switch {
	case commandExists("open"):
		cmd = exec.Command("open", path)
	case commandExists("xdg-open"):
		cmd = exec.Command("xdg-open", path)
	default:
		return errors.New("no supported PDF opener found")
	}
	return cmd.Run()
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func newerThan(a, b string) bool {
	ai, errA := os.Stat(a)
	bi, errB := os.Stat(b)
	return errA == nil && errB == nil && ai.ModTime().After(bi.ModTime())
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func defaultString(value, def string) string {
	if value == "" {
		return def
	}
	return value
}
