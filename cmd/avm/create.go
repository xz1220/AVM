package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/config"
)

type builtinPackage struct {
	Name             string   `yaml:"name"`
	Description      string   `yaml:"description"`
	Modes            []string `yaml:"modes"`
	DefaultName      string   `yaml:"default_name"`
	DefaultRuntime   string   `yaml:"default_runtime"`
	DefaultModel     string   `yaml:"default_model,omitempty"`
	DefaultReasoning string   `yaml:"default_reasoning"`
	Skills           []string `yaml:"skills,omitempty"`
	MCPs             []string `yaml:"mcps,omitempty"`
	Instructions     string   `yaml:"instructions,omitempty"`
}

type createOptions struct {
	Package   string
	Name      string
	Runtime   string
	Model     string
	Reasoning string
	Skills    []string
	MCPs      []string
	Scope     config.Scope
	Yes       bool
	NoInput   bool
}

func newCreateCommand() *cobra.Command {
	var opts createOptions
	var scope string

	cmd := &cobra.Command{
		Use:   "create [package]",
		Short: "Create an AVM agent profile from a package",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Package = args[0]
			}
			parsedScope, err := parseCreateScope(scope)
			if err != nil {
				return err
			}
			opts.Scope = parsedScope
			return runCreate(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.Name, "name", "", "agent profile name to create")
	cmd.Flags().StringVar(&opts.Runtime, "runtime", "", "preferred runtime for the created profile")
	cmd.Flags().StringVar(&opts.Model, "model", "", "model override")
	cmd.Flags().StringVar(&opts.Reasoning, "reasoning", "", "reasoning effort override")
	cmd.Flags().StringSliceVar(&opts.Skills, "skills", nil, "skills to attach")
	cmd.Flags().StringSliceVar(&opts.MCPs, "mcps", nil, "MCP servers to attach")
	cmd.Flags().StringVar(&scope, "scope", string(config.ScopeGlobal), "profile scope")
	cmd.Flags().BoolVar(&opts.Yes, "yes", false, "accept defaults and do not prompt")
	cmd.Flags().BoolVar(&opts.NoInput, "no-input", false, "fail instead of prompting")
	return cmd
}

func runCreate(cmd *cobra.Command, opts createOptions) error {
	if err := ensureInitialized(); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	reader := bufio.NewReader(cmd.InOrStdin())
	out := cmd.OutOrStdout()
	pkgName := strings.TrimSpace(opts.Package)
	if pkgName == "" {
		if opts.Yes {
			pkgName = "backend-coder"
		} else {
			pkgName, err = promptPackageName(reader, out, opts.NoInput)
			if err != nil {
				return err
			}
		}
	}

	pkg, ok := lookupBuiltinPackage(pkgName)
	if !ok {
		return fmt.Errorf("package %q not found; run avm package list", pkgName)
	}
	if !packageSupportsMode(pkg, "create") {
		return fmt.Errorf("package %q does not support create mode", pkg.Name)
	}

	values, err := resolveCreateValues(cmd, reader, out, pkg, opts)
	if err != nil {
		return err
	}
	if _, err := config.ReadAgent(values.Name, values.Scope, cwd); err == nil {
		return fmt.Errorf("agent %q already exists; choose another --name", values.Name)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	agent := agentFromBuiltinPackage(pkg, values)
	if err := config.WriteAgent(agent, values.Scope, cwd); err != nil {
		return err
	}

	fmt.Fprintf(out, "created agent %s from package %s\n", agent.Name, pkg.Name)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "To use it in this shell:")
	fmt.Fprintf(out, "  eval \"$(avm activate %s)\"\n", agent.Name)
	if runtimeCommand := runtimeStartCommand(agent.Runtime.Preferred); runtimeCommand != "" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Then start your runtime:")
		fmt.Fprintf(out, "  %s\n", runtimeCommand)
	}
	return nil
}

type resolvedCreateValues struct {
	Name      string
	Runtime   string
	Model     string
	Reasoning string
	Skills    []string
	MCPs      []string
	Scope     config.Scope
}

func resolveCreateValues(cmd *cobra.Command, reader *bufio.Reader, out io.Writer, pkg builtinPackage, opts createOptions) (resolvedCreateValues, error) {
	values := resolvedCreateValues{
		Name:      firstNonEmptyString(opts.Name, pkg.DefaultName, pkg.Name),
		Runtime:   firstNonEmptyString(opts.Runtime, pkg.DefaultRuntime, "codex"),
		Model:     firstNonEmptyString(opts.Model, pkg.DefaultModel),
		Reasoning: firstNonEmptyString(opts.Reasoning, pkg.DefaultReasoning, "medium"),
		Skills:    append([]string(nil), pkg.Skills...),
		MCPs:      append([]string(nil), pkg.MCPs...),
		Scope:     opts.Scope,
	}
	if cmd.Flags().Changed("skills") {
		values.Skills = normalizeStringList(opts.Skills)
	}
	if cmd.Flags().Changed("mcps") {
		values.MCPs = normalizeStringList(opts.MCPs)
	}

	if !opts.Yes {
		if opts.NoInput {
			return values, fmt.Errorf("create requires --yes or interactive input")
		}
		var err error
		values.Name, err = promptString(reader, out, "Agent name", values.Name)
		if err != nil {
			return values, err
		}
		values.Runtime, err = promptString(reader, out, "Runtime (codex, claude-code, opencode, cline, cursor)", values.Runtime)
		if err != nil {
			return values, err
		}
		if !isKnownRuntime(values.Runtime) {
			return values, fmt.Errorf("invalid runtime %q", values.Runtime)
		}
		confirmed, err := promptConfirm(reader, out, fmt.Sprintf("Create agent %q for %s", values.Name, values.Runtime), true)
		if err != nil {
			return values, err
		}
		if !confirmed {
			return values, fmt.Errorf("create cancelled")
		}
	}

	if !isKnownRuntime(values.Runtime) {
		return values, fmt.Errorf("invalid runtime %q", values.Runtime)
	}
	return values, nil
}

func agentFromBuiltinPackage(pkg builtinPackage, values resolvedCreateValues) *config.AgentProfile {
	return &config.AgentProfile{
		Name:        values.Name,
		Description: pkg.Description,
		SourceScope: string(values.Scope),
		Runtime: config.RuntimePreferences{
			Preferred: values.Runtime,
		},
		Identity: config.AgentIdentity{
			DisplayName: values.Name,
			Role:        pkg.Name,
		},
		Instructions: config.Instructions{
			Developer: pkg.Instructions,
		},
		ModelRun: config.ModelRun{
			Model:           values.Model,
			ReasoningEffort: values.Reasoning,
		},
		Capabilities: config.CapabilityRefs{
			Skills: values.Skills,
			MCPs:   values.MCPs,
		},
	}
}

func parseCreateScope(value string) (config.Scope, error) {
	if value == "" {
		return config.ScopeGlobal, nil
	}
	scope := config.Scope(value)
	switch scope {
	case config.ScopeGlobal, config.ScopeProject, config.ScopeLocal:
		return scope, nil
	default:
		return "", fmt.Errorf("invalid scope %q", value)
	}
}

func promptPackageName(reader *bufio.Reader, out io.Writer, noInput bool) (string, error) {
	if noInput {
		return "", fmt.Errorf("create requires a package name or --yes")
	}
	fmt.Fprintln(out, "Available packages:")
	for _, pkg := range listBuiltinPackages() {
		fmt.Fprintf(out, "  %s\t%s\n", pkg.Name, pkg.Description)
	}
	return promptString(reader, out, "Package", "backend-coder")
}

func promptString(reader *bufio.Reader, out io.Writer, label, defaultValue string) (string, error) {
	if defaultValue == "" {
		fmt.Fprintf(out, "%s: ", label)
	} else {
		fmt.Fprintf(out, "%s [%s]: ", label, defaultValue)
	}
	line, err := reader.ReadString('\n')
	if err != nil && !(err == io.EOF && line != "") {
		if err == io.EOF {
			return "", fmt.Errorf("input required for %s", strings.ToLower(label))
		}
		return "", err
	}
	value := strings.TrimSpace(line)
	if value == "" {
		value = defaultValue
	}
	return value, nil
}

func promptConfirm(reader *bufio.Reader, out io.Writer, label string, defaultYes bool) (bool, error) {
	suffix := " [Y/n]: "
	if !defaultYes {
		suffix = " [y/N]: "
	}
	fmt.Fprint(out, label+suffix)
	line, err := reader.ReadString('\n')
	if err != nil && !(err == io.EOF && line != "") {
		if err == io.EOF {
			return false, fmt.Errorf("input required for confirmation")
		}
		return false, err
	}
	value := strings.ToLower(strings.TrimSpace(line))
	if value == "" {
		return defaultYes, nil
	}
	switch value {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, fmt.Errorf("invalid confirmation %q", value)
	}
}

func runtimeStartCommand(runtime string) string {
	switch runtime {
	case "codex":
		return "codex"
	case "claude-code":
		return "claude"
	case "opencode":
		return "opencode"
	case "cline":
		return "cline"
	case "cursor":
		return "cursor ."
	default:
		return ""
	}
}

func listBuiltinPackages() []builtinPackage {
	packages := append([]builtinPackage(nil), builtinPackages...)
	sort.Slice(packages, func(i, j int) bool {
		return packages[i].Name < packages[j].Name
	})
	return packages
}

func lookupBuiltinPackage(name string) (builtinPackage, bool) {
	name = strings.TrimSpace(name)
	for _, pkg := range builtinPackages {
		if pkg.Name == name {
			return pkg, true
		}
	}
	return builtinPackage{}, false
}

func packageSupportsMode(pkg builtinPackage, mode string) bool {
	for _, candidate := range pkg.Modes {
		if candidate == mode {
			return true
		}
	}
	return false
}

var builtinPackages = []builtinPackage{
	{
		Name:             "backend-coder",
		Description:      "General backend coding agent with test-oriented defaults.",
		Modes:            []string{"create"},
		DefaultName:      "backend-coder",
		DefaultRuntime:   "codex",
		DefaultReasoning: "high",
		Skills:           []string{"git", "test"},
		Instructions:     "Focus on small backend changes, run targeted tests when practical, and keep implementation notes concise.",
	},
	{
		Name:             "reviewer",
		Description:      "Code review agent focused on risks, regressions, and missing tests.",
		Modes:            []string{"create"},
		DefaultName:      "reviewer",
		DefaultRuntime:   "claude-code",
		DefaultReasoning: "medium",
		Skills:           []string{"review", "test"},
		Instructions:     "Review changes for correctness, user-visible regressions, security issues, and missing tests before summarizing.",
	},
	{
		Name:             "writer",
		Description:      "Technical writing agent for docs, specs, and release notes.",
		Modes:            []string{"create"},
		DefaultName:      "writer",
		DefaultRuntime:   "codex",
		DefaultReasoning: "medium",
		Skills:           []string{"docs"},
		Instructions:     "Write clear technical prose, preserve factual nuance, and keep examples runnable where possible.",
	},
}
