package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	rcmcp "rootcause/internal/mcp"
)

// commandFormat selects per-agent file layout for generated slash commands.
type commandFormat string

const (
	formatClaudeCommand  commandFormat = "claude_command"
	formatCursorCommand  commandFormat = "cursor_command"
	formatCodexCommand   commandFormat = "codex_command"
	formatCopilotPrompt  commandFormat = "copilot_prompt"
	formatGenericMD      commandFormat = "generic_md"
)

type commandTarget struct {
	Agent  string
	Dir    string
	Format commandFormat
}

var commandTargets = map[string]commandTarget{
	"claude":         {Agent: "Claude Code", Dir: ".claude/commands", Format: formatClaudeCommand},
	"cursor":         {Agent: "Cursor", Dir: ".cursor/commands", Format: formatCursorCommand},
	"codex":          {Agent: "Codex", Dir: ".codex/commands", Format: formatCodexCommand},
	"copilot":        {Agent: "GitHub Copilot", Dir: ".github/prompts", Format: formatCopilotPrompt},
	"github-copilot": {Agent: "GitHub Copilot", Dir: ".github/prompts", Format: formatCopilotPrompt},
	"gemini":         {Agent: "Gemini CLI", Dir: ".gemini/commands", Format: formatGenericMD},
	"gemini-cli":     {Agent: "Gemini CLI", Dir: ".gemini/commands", Format: formatGenericMD},
	"opencode":       {Agent: "OpenCode", Dir: ".opencode/commands", Format: formatGenericMD},
	"windsurf":       {Agent: "Windsurf", Dir: ".windsurf/commands", Format: formatGenericMD},
	"aider":          {Agent: "Aider", Dir: ".aider/commands", Format: formatGenericMD},
}

var canonicalCommandAgentKeys = []string{
	"claude",
	"cursor",
	"codex",
	"copilot",
	"gemini",
	"opencode",
	"windsurf",
	"aider",
}


func selectedPrompts(specs []rcmcp.PromptSpec, filters []string) ([]rcmcp.PromptSpec, error) {
	if len(filters) == 0 {
		out := append([]rcmcp.PromptSpec{}, specs...)
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return out, nil
	}
	allowed := map[string]struct{}{}
	for _, f := range filters {
		trimmed := strings.TrimSpace(strings.ToLower(f))
		if trimmed != "" {
			allowed[trimmed] = struct{}{}
		}
	}
	var out []rcmcp.PromptSpec
	for _, p := range specs {
		if _, ok := allowed[strings.ToLower(p.Name)]; ok {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no matching prompts for filters: %v", filters)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func syncCommandsForTarget(projectDir string, target commandTarget, prompts []rcmcp.PromptSpec, overwrite bool, dryRun bool) (int, string, error) {
	expanded, err := expandPromptHome(projectDir)
	if err != nil {
		return 0, "", err
	}
	absProject, err := filepath.Abs(expanded)
	if err != nil {
		return 0, "", err
	}
	destRoot := filepath.Join(absProject, filepath.FromSlash(target.Dir))
	if !dryRun {
		if err := os.MkdirAll(destRoot, 0o755); err != nil {
			return 0, "", fmt.Errorf("create destination dir: %w", err)
		}
	}
	count := 0
	for _, p := range prompts {
		fileName := commandFileName(p.Name, target.Format)
		destFile := filepath.Join(destRoot, fileName)
		body := renderCommandFile(p, target.Format)
		if !dryRun {
			if !overwrite {
				if _, err := os.Stat(destFile); err == nil {
					continue
				}
			}
			if err := os.WriteFile(destFile, []byte(body), 0o644); err != nil {
				return count, destRoot, fmt.Errorf("write %s: %w", destFile, err)
			}
		}
		count++
	}
	return count, destRoot, nil
}

func commandFileName(promptName string, format commandFormat) string {
	slug := slugifyPromptName(promptName)
	switch format {
	case formatCopilotPrompt:
		return slug + ".prompt.md"
	default:
		return slug + ".md"
	}
}

// slugifyPromptName converts MCP prompt names (snake_case) into the kebab-case
// convention used by client slash commands. troubleshoot_workload -> troubleshoot-workload.
func slugifyPromptName(name string) string {
	return strings.ReplaceAll(strings.TrimSpace(name), "_", "-")
}

// renderCommandFile produces the agent-native body for a prompt. It substitutes
// positional placeholders ($1, $2, ...) for {{name}} tokens (in argument order)
// and surfaces template defaults so values are still meaningful when the user
// omits an optional argument.
func renderCommandFile(spec rcmcp.PromptSpec, format commandFormat) string {
	body, defaults := substitutePositional(spec.Template, spec.Arguments)
	desc := strings.TrimSpace(spec.Description)
	if desc == "" {
		desc = spec.Name
	}
	hint := argumentHint(spec.Arguments)

	var b strings.Builder
	switch format {
	case formatClaudeCommand, formatCursorCommand, formatCodexCommand, formatGenericMD:
		b.WriteString("---\n")
		fmt.Fprintf(&b, "description: %s\n", escapeFrontMatter(desc))
		if hint != "" {
			fmt.Fprintf(&b, "argument-hint: %s\n", escapeFrontMatter(hint))
		}
		b.WriteString("---\n\n")
	case formatCopilotPrompt:
		b.WriteString("---\n")
		b.WriteString("mode: agent\n")
		fmt.Fprintf(&b, "description: %s\n", escapeFrontMatter(desc))
		if hint != "" {
			fmt.Fprintf(&b, "argument-hint: %s\n", escapeFrontMatter(hint))
		}
		b.WriteString("---\n\n")
	}

	if block := defaultsSection(spec.Arguments, defaults); block != "" {
		b.WriteString(block)
		b.WriteString("\n")
	}
	b.WriteString(strings.TrimRight(body, "\n"))
	b.WriteString("\n")
	return b.String()
}

// substitutePositional replaces every {{name}} or {{name|default}} token in the
// template with the positional argument $N where N matches the argument's
// index (1-based) in spec.Arguments. Defaults extracted from `{{name|default}}`
// tokens are returned in a map keyed by argument name so the caller can render
// a Defaults block. Unknown tokens are left untouched.
func substitutePositional(template string, args []rcmcp.PromptArgument) (string, map[string]string) {
	positions := map[string]int{}
	for i, a := range args {
		positions[a.Name] = i + 1
	}
	defaults := map[string]string{}
	out := template
	cursor := 0
	for {
		start := strings.Index(out[cursor:], "{{")
		if start < 0 {
			break
		}
		start += cursor
		end := strings.Index(out[start+2:], "}}")
		if end < 0 {
			break
		}
		end += start + 2
		token := strings.TrimSpace(out[start+2 : end])
		parts := strings.SplitN(token, "|", 2)
		key := strings.TrimSpace(parts[0])
		repl := "{{" + token + "}}"
		if pos, ok := positions[key]; ok {
			repl = fmt.Sprintf("$%d", pos)
			if len(parts) == 2 {
				def := strings.TrimSpace(parts[1])
				if def != "" {
					defaults[key] = def
				}
			}
		}
		out = out[:start] + repl + out[end+2:]
		cursor = start + len(repl)
	}
	return out, defaults
}

func argumentHint(args []rcmcp.PromptArgument) string {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		if a.Required {
			parts = append(parts, fmt.Sprintf("<%s>", a.Name))
		} else {
			parts = append(parts, fmt.Sprintf("[%s]", a.Name))
		}
	}
	return strings.Join(parts, " ")
}

// defaultsSection emits a "Defaults" block so the agent applies fallback values
// when optional positional args are omitted. Required args without defaults are
// excluded (they're guaranteed present at invocation time). The defaults map
// is the one returned by substitutePositional and reflects what the template
// itself declared via `{{name|default}}`.
func defaultsSection(args []rcmcp.PromptArgument, defaults map[string]string) string {
	if len(args) == 0 || len(defaults) == 0 {
		return ""
	}
	var b strings.Builder
	hasDefault := false
	for i, a := range args {
		def, ok := defaults[a.Name]
		if !ok || def == "" {
			continue
		}
		if !hasDefault {
			b.WriteString("If a positional argument is empty, treat it as the default:\n")
			hasDefault = true
		}
		fmt.Fprintf(&b, "- $%d (%s) → %s\n", i+1, a.Name, def)
	}
	if !hasDefault {
		return ""
	}
	return b.String()
}

func escapeFrontMatter(s string) string {
	// YAML-safe single-line value: replace newlines and wrap in quotes only
	// when necessary. For descriptions in our prompts, simple replacement is
	// enough — they're already one line.
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}

// expandPromptHome expands a leading '~' to the user's home directory.
func expandPromptHome(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return ".", nil
	}
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return home, nil
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}
