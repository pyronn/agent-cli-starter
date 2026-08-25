package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
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
)

var (
	cliNamePattern     = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	envPrefixPattern   = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	appNamePattern     = regexp.MustCompile(`appName\s*=\s*"([^"]+)"`)
	appDescriptionLine = regexp.MustCompile(`(?m)^(\s*appDescription\s*=\s*)"(?:\\.|[^"])*"`)
	envEndpointPattern = regexp.MustCompile(`"([A-Z][A-Z0-9_]*)_ENDPOINT"`)
)

type projectState struct {
	name      string
	module    string
	envPrefix string
}

type settings struct {
	root        string
	name        string
	module      string
	envPrefix   string
	description string
	yes         bool
	force       bool
	dryRun      bool
	noVerify    bool
}

type fileEdit struct {
	path string
	data []byte
	mode fs.FileMode
}

type changePlan struct {
	edits      []fileEdit
	oldCommand string
	newCommand string
	oldSkill   string
	newSkill   string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	options, err := parseFlags(args, stderr)
	if err != nil {
		return 2
	}

	root, err := filepath.Abs(options.root)
	if err != nil {
		fmt.Fprintf(stderr, "error: resolve project root: %v\n", err)
		return 1
	}
	options.root = root

	current, err := discoverProject(root)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	reader := bufio.NewReader(stdin)
	if options.name == "" {
		options.name, err = prompt(reader, stdout, "CLI name", current.name)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
	}
	if options.module == "" {
		options.module, err = prompt(reader, stdout, "Go module path", current.module)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
	}
	if options.envPrefix == "" {
		options.envPrefix = deriveEnvPrefix(options.name)
	}
	if options.description == "" {
		options.description = fmt.Sprintf("%s command-line application", options.name)
	}
	if err := validateSettings(options); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}

	if !options.force {
		dirty, err := isGitDirty(root)
		if err != nil {
			fmt.Fprintf(stderr, "error: inspect Git worktree: %v\n", err)
			return 1
		}
		if dirty {
			fmt.Fprintln(stderr, "error: Git worktree has uncommitted changes; commit them first or pass --force")
			return 1
		}
	}

	printSummary(stdout, current, options)
	if !options.yes && !options.dryRun {
		answer, err := prompt(reader, stdout, "Apply these changes? [y/N]", "N")
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
		if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
			fmt.Fprintln(stdout, "Initialization cancelled.")
			return 0
		}
	}

	plan, err := buildPlan(root, current, options)
	if err != nil {
		fmt.Fprintf(stderr, "error: create initialization plan: %v\n", err)
		return 1
	}
	if options.dryRun {
		printPlan(stdout, root, plan)
		return 0
	}
	if err := applyPlan(plan); err != nil {
		fmt.Fprintf(stderr, "error: apply initialization plan: %v\n", err)
		return 1
	}

	if plan.oldCommand != plan.newCommand {
		fmt.Fprintf(stdout, "Updated %d files and renamed %s to %s.\n",
			len(plan.edits),
			relativePath(root, plan.oldCommand),
			relativePath(root, plan.newCommand),
		)
	} else {
		fmt.Fprintf(stdout, "Updated %d files.\n", len(plan.edits))
	}
	if plan.oldSkill != plan.newSkill {
		fmt.Fprintf(stdout, "Renamed %s to %s.\n",
			relativePath(root, plan.oldSkill),
			relativePath(root, plan.newSkill),
		)
	}
	if !options.noVerify {
		if err := execute(stdout, stderr, root, "go", "mod", "tidy"); err != nil {
			fmt.Fprintf(stderr, "error: initialization was applied, but go mod tidy failed: %v\n", err)
			return 1
		}
		if err := execute(stdout, stderr, root, "go", "test", "./..."); err != nil {
			fmt.Fprintf(stderr, "error: initialization was applied, but tests failed: %v\n", err)
			return 1
		}
	}

	fmt.Fprintf(stdout, "\nInitialization complete. Build your CLI with:\n  go build ./cmd/%s\n", options.name)
	return 0
}

func parseFlags(args []string, stderr io.Writer) (settings, error) {
	var options settings
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&options.root, "root", ".", "template repository root")
	flags.StringVar(&options.name, "name", "", "new executable and CLI name, for example acmectl")
	flags.StringVar(&options.module, "module", "", "new Go module path, for example github.com/acme/acmectl")
	flags.StringVar(&options.envPrefix, "env-prefix", "", "environment prefix; defaults to the uppercased CLI name")
	flags.StringVar(&options.description, "description", "", "short CLI and README description")
	flags.BoolVar(&options.yes, "yes", false, "apply without an interactive confirmation")
	flags.BoolVar(&options.force, "force", false, "allow initialization with a dirty Git worktree")
	flags.BoolVar(&options.dryRun, "dry-run", false, "show the planned changes without writing files")
	flags.BoolVar(&options.noVerify, "no-verify", false, "skip go mod tidy and go test after initialization")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Initialize this template with your own CLI identity.")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Usage:")
		fmt.Fprintln(stderr, "  go run ./tools/init --name NAME --module MODULE [options]")
		fmt.Fprintln(stderr)
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return settings{}, err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return settings{}, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	return options, nil
}

func discoverProject(root string) (projectState, error) {
	goModPath := filepath.Join(root, "go.mod")
	goMod, err := os.ReadFile(goModPath)
	if err != nil {
		return projectState{}, fmt.Errorf("read %s: %w", goModPath, err)
	}
	module := ""
	for _, line := range strings.Split(string(goMod), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			module = strings.TrimSpace(strings.TrimPrefix(line, "module "))
			break
		}
	}
	if module == "" {
		return projectState{}, fmt.Errorf("go.mod does not declare a module")
	}

	rootSource, err := os.ReadFile(filepath.Join(root, "internal", "cli", "root.go"))
	if err != nil {
		return projectState{}, fmt.Errorf("read CLI root command: %w", err)
	}
	nameMatch := appNamePattern.FindSubmatch(rootSource)
	if len(nameMatch) != 2 {
		return projectState{}, fmt.Errorf("cannot find appName in internal/cli/root.go")
	}
	name := string(nameMatch[1])
	if _, err := os.Stat(filepath.Join(root, "cmd", name, "main.go")); err != nil {
		return projectState{}, fmt.Errorf("expected command entry cmd/%s/main.go: %w", name, err)
	}

	configSource, err := os.ReadFile(filepath.Join(root, "internal", "config", "config.go"))
	if err != nil {
		return projectState{}, fmt.Errorf("read configuration source: %w", err)
	}
	envMatch := envEndpointPattern.FindSubmatch(configSource)
	if len(envMatch) != 2 {
		return projectState{}, fmt.Errorf("cannot discover environment prefix from internal/config/config.go")
	}

	return projectState{name: name, module: module, envPrefix: string(envMatch[1])}, nil
}

func validateSettings(options settings) error {
	if !cliNamePattern.MatchString(options.name) {
		return fmt.Errorf("--name must match %s", cliNamePattern)
	}
	if err := validateModulePath(options.module); err != nil {
		return fmt.Errorf("--module %w", err)
	}
	if !envPrefixPattern.MatchString(options.envPrefix) {
		return fmt.Errorf("--env-prefix must match %s", envPrefixPattern)
	}
	if strings.ContainsAny(options.description, "\r\n") {
		return fmt.Errorf("--description must be a single line")
	}
	if strings.TrimSpace(options.description) == "" {
		return fmt.Errorf("--description cannot be empty")
	}
	return nil
}

func validateModulePath(module string) error {
	if module == "" {
		return fmt.Errorf("cannot be empty")
	}
	if strings.ContainsAny(module, " \t\r\n\\@") {
		return fmt.Errorf("contains an invalid character")
	}
	if strings.HasPrefix(module, "/") || strings.HasSuffix(module, "/") || strings.Contains(module, "//") {
		return fmt.Errorf("must be a slash-separated import path")
	}
	for _, part := range strings.Split(module, "/") {
		if part == "." || part == ".." || part == "" {
			return fmt.Errorf("contains an invalid path segment")
		}
	}
	return nil
}

func deriveEnvPrefix(name string) string {
	var result strings.Builder
	lastUnderscore := false
	for _, char := range strings.ToUpper(name) {
		if char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' {
			result.WriteRune(char)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			result.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(result.String(), "_")
}

func buildPlan(root string, current projectState, options settings) (changePlan, error) {
	oldCommand := filepath.Join(root, "cmd", current.name)
	newCommand := filepath.Join(root, "cmd", options.name)
	oldSkill := filepath.Join(root, "skills", current.name)
	newSkill := filepath.Join(root, "skills", options.name)
	if err := validateRenameTarget(oldCommand, newCommand, "command"); err != nil {
		return changePlan{}, err
	}
	if err := validateRenameTarget(oldSkill, newSkill, "skill"); err != nil {
		return changePlan{}, err
	}

	var edits []fileEdit
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && shouldSkipDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !shouldEditFile(path) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		updated := replaceIdentity(data, current, options)
		relative := relativePath(root, path)
		switch filepath.ToSlash(relative) {
		case "internal/cli/root.go":
			updated = updateDescription(updated, options.description)
		case "README.md":
			updated = updateReadme(updated, options.name, options.description)
		}
		if !bytes.Equal(data, updated) {
			edits = append(edits, fileEdit{path: path, data: updated, mode: info.Mode().Perm()})
		}
		return nil
	})
	if err != nil {
		return changePlan{}, err
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].path < edits[j].path })
	return changePlan{
		edits: edits, oldCommand: oldCommand, newCommand: newCommand,
		oldSkill: oldSkill, newSkill: newSkill,
	}, nil
}

func validateRenameTarget(source, target, label string) error {
	if source == target {
		return nil
	}
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("expected %s directory %s: %w", label, source, err)
	}
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("target %s directory already exists: %s", label, target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect target %s directory: %w", label, err)
	}
	return nil
}

func replaceIdentity(data []byte, current projectState, options settings) []byte {
	replacements := [][2]string{
		{current.module, options.module},
		{githubRepository(current.module), githubRepository(options.module)},
		{current.envPrefix, options.envPrefix},
		{current.name, options.name},
	}
	tokens := [][]byte{
		{0, 1, 0},
		{0, 2, 0},
		{0, 3, 0},
		{0, 4, 0},
	}
	result := append([]byte(nil), data...)
	for index, replacement := range replacements {
		if replacement[0] != "" {
			result = bytes.ReplaceAll(result, []byte(replacement[0]), tokens[index])
		}
	}
	for index, replacement := range replacements {
		result = bytes.ReplaceAll(result, tokens[index], []byte(replacement[1]))
	}
	return result
}

func githubRepository(module string) string {
	return strings.TrimPrefix(module, "github.com/")
}

func updateDescription(data []byte, description string) []byte {
	return appDescriptionLine.ReplaceAllFunc(data, func(match []byte) []byte {
		quote := bytes.IndexByte(match, '"')
		if quote == -1 {
			return match
		}
		result := append([]byte(nil), match[:quote]...)
		return append(result, strconv.Quote(description)...)
	})
}

func updateReadme(data []byte, name, description string) []byte {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) == 0 {
		return data
	}
	lines[0] = "# " + name
	start := 1
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := start
	for end < len(lines) && strings.TrimSpace(lines[end]) != "" {
		end++
	}
	for end < len(lines) && strings.TrimSpace(lines[end]) == "" {
		end++
	}
	replacement := []string{"", description, ""}
	lines = append(append(append([]string{}, lines[:1]...), replacement...), lines[end:]...)
	return []byte(strings.Join(lines, "\n"))
}

func shouldSkipDirectory(name string) bool {
	switch name {
	case ".git", ".cache", ".idea", "dist", "vendor":
		return true
	default:
		return false
	}
}

func shouldEditFile(path string) bool {
	switch filepath.Base(path) {
	case "go.mod", "go.sum":
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".md", ".yml", ".yaml", ".ps1", ".sh", ".json", ".toml":
		return true
	default:
		return false
	}
}

func applyPlan(plan changePlan) error {
	for _, edit := range plan.edits {
		if err := os.WriteFile(edit.path, edit.data, edit.mode); err != nil {
			return fmt.Errorf("write %s: %w", edit.path, err)
		}
	}
	if plan.oldCommand != plan.newCommand {
		if err := os.Rename(plan.oldCommand, plan.newCommand); err != nil {
			return fmt.Errorf("rename command directory: %w", err)
		}
	}
	if plan.oldSkill != plan.newSkill {
		if err := os.Rename(plan.oldSkill, plan.newSkill); err != nil {
			return fmt.Errorf("rename skill directory: %w", err)
		}
	}
	return nil
}

func isGitDirty(root string) (bool, error) {
	command := exec.Command("git", "-C", root, "status", "--porcelain", "--untracked-files=normal")
	output, err := command.CombinedOutput()
	if err == nil {
		return len(bytes.TrimSpace(output)) > 0, nil
	}
	var pathError *exec.Error
	if errors.As(err, &pathError) {
		return false, nil
	}
	if bytes.Contains(output, []byte("not a git repository")) {
		return false, nil
	}
	return false, fmt.Errorf("%w: %s", err, bytes.TrimSpace(output))
}

func execute(stdout, stderr io.Writer, root, name string, args ...string) error {
	fmt.Fprintf(stdout, "\n> %s %s\n", name, strings.Join(args, " "))
	command := exec.Command(name, args...)
	command.Dir = root
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

func prompt(reader *bufio.Reader, output io.Writer, label, defaultValue string) (string, error) {
	if defaultValue == "" {
		fmt.Fprintf(output, "%s: ", label)
	} else {
		fmt.Fprintf(output, "%s [%s]: ", label, defaultValue)
	}
	value, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		value = defaultValue
	}
	if value == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	return value, nil
}

func printSummary(output io.Writer, current projectState, options settings) {
	fmt.Fprintln(output, "Template initialization:")
	fmt.Fprintf(output, "  CLI name:          %s -> %s\n", current.name, options.name)
	fmt.Fprintf(output, "  Go module:         %s -> %s\n", current.module, options.module)
	fmt.Fprintf(output, "  Environment prefix: %s -> %s\n", current.envPrefix, options.envPrefix)
	fmt.Fprintf(output, "  Description:       %s\n", options.description)
}

func printPlan(output io.Writer, root string, plan changePlan) {
	fmt.Fprintf(output, "\nDry run: %d files would be updated:\n", len(plan.edits))
	for _, edit := range plan.edits {
		fmt.Fprintf(output, "  %s\n", relativePath(root, edit.path))
	}
	if plan.oldCommand != plan.newCommand {
		fmt.Fprintf(output, "  rename %s -> %s\n", relativePath(root, plan.oldCommand), relativePath(root, plan.newCommand))
	}
	if plan.oldSkill != plan.newSkill {
		fmt.Fprintf(output, "  rename %s -> %s\n", relativePath(root, plan.oldSkill), relativePath(root, plan.newSkill))
	}
}

func relativePath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(relative)
}
