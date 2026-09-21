package harness

import (
	"bytes"
	"dossier/assets"
	"dossier/internal/core"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ClaudeCodeHarness implements capability detection and installation for Claude Code.
const claudeSkillInstruction = "Dossier: ALWAYS use the dossier_session tool to identify or switch to your active dossier when starting work. Do NOT attempt to bypass MCP."

type ClaudeCodeHarness struct {
	dossierHome string
}

// NewClaudeCodeHarness instantiates the Claude Code harness.
func NewClaudeCodeHarness(dossierHome string) *ClaudeCodeHarness {
	return &ClaudeCodeHarness{
		dossierHome: dossierHome,
	}
}

// Name returns the identifier of the harness.
func (c *ClaudeCodeHarness) Name() string {
	return "claude-code"
}

// Detect checks if Claude Code is installed and returns its capabilities.
func (c *ClaudeCodeHarness) Detect() (core.Capabilities, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return core.Capabilities{}, err
	}

	claudeJSONPath := filepath.Join(home, ".claude.json")
	settingsPath := filepath.Join(home, ".claude", "settings.json")

	_, err1 := os.Stat(claudeJSONPath)
	_, err2 := os.Stat(settingsPath)

	if os.IsNotExist(err1) && os.IsNotExist(err2) {
		return core.Capabilities{}, nil
	}

	return core.Capabilities{
		MCP:               true,
		SessionStartHook:  true,
		SessionEndHook:    true,
		PreCompactionHook: true,
		TranscriptCapture: true,
		Installed:         true,
		// Claude Code sets CLAUDE_CODE_SESSION_ID in every session's process
		// environment (verified; ADR 0003), so identity needs no bridge.
		SessionIdentity: true,
	}, nil
}

func quoteCommandPath(path string) string {
	return `"` + strings.ReplaceAll(path, `"`, `\"`) + `"`
}

// isClaudeCodeMCPConfigured checks if dossier is registered correctly in mcpServers.
func isClaudeCodeMCPConfigured(configMap map[string]any, stablePath string) bool {
	mcpServersVal, ok := configMap["mcpServers"]
	if !ok {
		return false
	}
	mcpServers, ok := mcpServersVal.(map[string]any)
	if !ok {
		return false
	}
	dossierVal, ok := mcpServers["dossier"]
	if !ok {
		return false
	}
	dossierMap, ok := dossierVal.(map[string]any)
	if !ok {
		return false
	}
	cmd, _ := dossierMap["command"].(string)
	if cmd != stablePath {
		return false
	}
	argsVal, ok := dossierMap["args"]
	if !ok {
		return false
	}
	args, ok := argsVal.([]any)
	if !ok {
		return false
	}
	if len(args) != 2 || args[0] != "mcp" || args[1] != "serve" {
		return false
	}
	return true
}

// Install configures lifecycle hooks in settings.json (or .claude.json as fallback) and MCP registration ALWAYS in .claude.json.
func (c *ClaudeCodeHarness) Install(opts core.InstallOpts) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	claudeJSONPath := filepath.Join(home, ".claude.json")
	settingsPath := filepath.Join(home, ".claude", "settings.json")

	// Determine where hooks should be written
	var hooksPath string
	if _, err := os.Stat(settingsPath); err == nil {
		hooksPath = settingsPath
	} else if _, err := os.Stat(claudeJSONPath); err == nil {
		hooksPath = claudeJSONPath
	} else {
		// If neither exists, skip installation entirely
		return nil
	}

	if resolved, err := filepath.EvalSymlinks(hooksPath); err == nil {
		hooksPath = resolved
	}
	if resolved, err := filepath.EvalSymlinks(claudeJSONPath); err == nil {
		claudeJSONPath = resolved
	}

	stablePath := opts.StableBinaryPath
	if stablePath == "" {
		executable, err := os.Executable()
		if err != nil {
			executable = "dossier"
		}
		stablePath = executable
	}
	// Hook commands are shell strings rather than argv arrays. Quote the
	// executable so Windows paths (and Unix paths with spaces) stay one token.
	execCmd := fmt.Sprintf("%s hook", quoteCommandPath(stablePath))

	// 1. Read hooks configuration
	hooksData, err := os.ReadFile(hooksPath)
	if err != nil {
		return fmt.Errorf("failed to read Claude Code hooks config: %w", err)
	}
	var hooksConfigMap map[string]any
	if len(hooksData) > 0 {
		if err := json.Unmarshal(hooksData, &hooksConfigMap); err != nil {
			hooksConfigMap = make(map[string]any)
		}
	} else {
		hooksConfigMap = make(map[string]any)
	}

	hooksVal, ok := hooksConfigMap["hooks"]
	var hooksMap map[string]any
	if ok {
		hooksMap, _ = hooksVal.(map[string]any)
	}
	if hooksMap == nil {
		hooksMap = make(map[string]any)
	}

	startCmd := execCmd + " session-start"
	endCmd := execCmd + " session-end"
	preCompactCmd := execCmd + " pre-compaction"

	hooksOk := isHookConfigured(hooksMap["SessionStart"], startCmd) &&
		isHookConfigured(hooksMap["SessionEnd"], endCmd) &&
		isHookConfigured(hooksMap["PreCompact"], preCompactCmd)

	// Migration: older versions mistakenly wrote the dossier MCP entry into the
	// hooks file (settings.json). MCP servers belong in ~/.claude.json; strip any
	// stale entry from the hooks file so it doesn't linger as dead config.
	staleMCPInHooks := false
	if hooksPath != claudeJSONPath {
		if msVal, ok := hooksConfigMap["mcpServers"].(map[string]any); ok {
			if _, has := msVal["dossier"]; has {
				delete(msVal, "dossier")
				if len(msVal) == 0 {
					delete(hooksConfigMap, "mcpServers")
				} else {
					hooksConfigMap["mcpServers"] = msVal
				}
				staleMCPInHooks = true
			}
		}
	}

	// 2. Read MCP configuration (ALWAYS ~/.claude.json)
	var mcpData []byte
	mcpExists := false
	if _, err := os.Stat(claudeJSONPath); err == nil {
		mcpExists = true
		mcpData, err = os.ReadFile(claudeJSONPath)
		if err != nil {
			return fmt.Errorf("failed to read Claude Code MCP config: %w", err)
		}
	}

	var mcpConfigMap map[string]any
	if len(mcpData) > 0 {
		if err := json.Unmarshal(mcpData, &mcpConfigMap); err != nil {
			mcpConfigMap = make(map[string]any)
		}
	} else {
		mcpConfigMap = make(map[string]any)
	}

	mcpOk := isClaudeCodeMCPConfigured(mcpConfigMap, stablePath)

	// Check for Dossier skill custom instruction
	hasSkill := false
	if ci, ok := hooksConfigMap["customInstructions"]; ok {
		if arr, ok := ci.([]any); ok {
			for _, v := range arr {
				if s, ok := v.(string); ok && strings.Contains(s, "Dossier") {
					hasSkill = true
					break
				}
			}
		} else if s, ok := ci.(string); ok && strings.Contains(s, "Dossier") {
			hasSkill = true
		}
	}

	// Check whether the dossier-delegate Claude Code Skill is already installed
	// with byte-identical content to the embedded asset. This is unrelated to
	// hasSkill above (that's the customInstructions trigger string) — the
	// dossier-delegate skill is explicit-invocation-only and must never be
	// wired into customInstructions/session-start auto-injection.
	delegateSkillContent, err := assets.FS.ReadFile("dossier-delegate-skill.md")
	if err != nil {
		return fmt.Errorf("failed to read embedded dossier-delegate skill asset: %w", err)
	}
	delegateSkillPath := filepath.Join(home, ".claude", "skills", "dossier-delegate", "SKILL.md")
	delegateSkillOk := false
	existingDelegateSkill, delegateSkillErr := os.ReadFile(delegateSkillPath)
	if delegateSkillErr == nil && bytes.Equal(existingDelegateSkill, delegateSkillContent) {
		delegateSkillOk = true
	}

	sparkSkillContent, err := assets.FS.ReadFile("spark-skill.md")
	if err != nil {
		return fmt.Errorf("failed to read embedded spark skill asset: %w", err)
	}
	sparkSkillPath := filepath.Join(home, ".claude", "skills", "spark", "SKILL.md")
	sparkSkillOk := managedAssetInstalled(sparkSkillPath, sparkSkillContent)

	if hooksOk && mcpOk && !staleMCPInHooks && hasSkill && delegateSkillOk && sparkSkillOk {
		return nil
	}

	if !opts.YesToAll {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			// Not a terminal, abort interactive prompt
			return nil
		}

		fmt.Printf("Configure Claude Code integration (hooks + MCP server)? [y/N]: ")
		var response string
		_, _ = fmt.Scanln(&response)
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			return nil
		}
	}

	timestamp := time.Now().Unix()

	// Backup hooks path if hooks are changing (or stale MCP is being stripped) and file exists
	if (!hooksOk || staleMCPInHooks) && len(hooksData) > 0 {
		backupPath := fmt.Sprintf("%s.%d.bak", hooksPath, timestamp)
		if err := os.WriteFile(backupPath, hooksData, 0644); err != nil {
			return fmt.Errorf("failed to create hooks config backup: %w", err)
		}
	}

	// Backup MCP path if MCP is changing and file exists
	if !mcpOk && mcpExists && len(mcpData) > 0 {
		// Avoid backing up the same file twice if hooksPath == claudeJSONPath
		if hooksPath != claudeJSONPath {
			backupPath := fmt.Sprintf("%s.%d.bak", claudeJSONPath, timestamp)
			if err := os.WriteFile(backupPath, mcpData, 0644); err != nil {
				return fmt.Errorf("failed to create MCP config backup: %w", err)
			}
		}
	}

	// Update and write hooks if needed (hook change or stale-MCP migration)
	if !hooksOk || staleMCPInHooks {
		if !hooksOk {
			hooksMap["SessionStart"] = updateHookArray(hooksMap["SessionStart"], startCmd, "hook session-start")
			hooksMap["SessionEnd"] = updateHookArray(hooksMap["SessionEnd"], endCmd, "hook session-end")
			hooksMap["PreCompact"] = updateHookArray(hooksMap["PreCompact"], preCompactCmd, "hook pre-compaction")
			hooksConfigMap["hooks"] = hooksMap
		}

		// Inject custom instruction for Dossier usage
		if !hasSkill {
			var customInst []string
			if ci, ok := hooksConfigMap["customInstructions"]; ok {
				if arr, ok := ci.([]any); ok {
					for _, v := range arr {
						if s, ok := v.(string); ok {
							customInst = append(customInst, s)
						}
					}
				} else if s, ok := ci.(string); ok {
					customInst = append(customInst, s)
				}
			}
			customInst = append(customInst, claudeSkillInstruction)
			hooksConfigMap["customInstructions"] = customInst
		}

		// If hooksPath == claudeJSONPath, we merge MCP updates into the same map before writing
		if hooksPath == claudeJSONPath {
			mcpConfigMap = hooksConfigMap
		} else {
			newHooksData, err := json.MarshalIndent(hooksConfigMap, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal hooks config: %w", err)
			}
			if err := os.WriteFile(hooksPath, newHooksData, 0644); err != nil {
				return fmt.Errorf("failed to write hooks config: %w", err)
			}
		}
	}

	// Update and write MCP if needed
	if !mcpOk || hooksPath == claudeJSONPath {
		mcpServersMap := make(map[string]any)
		if mVal, ok := mcpConfigMap["mcpServers"]; ok {
			if m, ok := mVal.(map[string]any); ok {
				mcpServersMap = m
			}
		}
		mcpServersMap["dossier"] = map[string]any{
			"type":    "stdio",
			"command": stablePath,
			"args":    []any{"mcp", "serve"},
		}
		mcpConfigMap["mcpServers"] = mcpServersMap

		newMcpData, err := json.MarshalIndent(mcpConfigMap, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal MCP config: %w", err)
		}
		if err := os.WriteFile(claudeJSONPath, newMcpData, 0644); err != nil {
			return fmt.Errorf("failed to write MCP config: %w", err)
		}
	}

	// Install (or update) the Claude Code Skills. These are standalone file
	// writes, independent of the hooks/MCP state above.
	if !delegateSkillOk {
		delegateSkillDir := filepath.Dir(delegateSkillPath)
		if err := os.MkdirAll(delegateSkillDir, 0755); err != nil {
			return fmt.Errorf("failed to create dossier-delegate skill directory: %w", err)
		}
		if delegateSkillErr == nil && len(existingDelegateSkill) > 0 {
			backupPath := fmt.Sprintf("%s.%d.bak", delegateSkillPath, timestamp)
			if err := os.WriteFile(backupPath, existingDelegateSkill, 0644); err != nil {
				return fmt.Errorf("failed to create dossier-delegate skill backup: %w", err)
			}
		}
		if err := os.WriteFile(delegateSkillPath, delegateSkillContent, 0644); err != nil {
			return fmt.Errorf("failed to write dossier-delegate skill: %w", err)
		}
	}
	if !sparkSkillOk {
		if err := installManagedAsset(sparkSkillPath, sparkSkillContent, timestamp); err != nil {
			return fmt.Errorf("failed to install spark skill: %w", err)
		}
	}

	return nil
}

func isDossierHookCommand(command string) bool {
	command = strings.TrimSpace(command)
	for _, suffix := range []string{
		" hook session-start",
		" hook session-end",
		" hook pre-compaction",
	} {
		if strings.HasSuffix(command, suffix) {
			return true
		}
	}
	return false
}

func removeDossierHooks(value any) (any, bool) {
	matchers, ok := value.([]any)
	if !ok {
		return value, false
	}
	changed := false
	keptMatchers := make([]any, 0, len(matchers))
	for _, matcherValue := range matchers {
		matcher, ok := matcherValue.(map[string]any)
		if !ok {
			keptMatchers = append(keptMatchers, matcherValue)
			continue
		}
		hooks, ok := matcher["hooks"].([]any)
		if !ok {
			keptMatchers = append(keptMatchers, matcherValue)
			continue
		}
		keptHooks := make([]any, 0, len(hooks))
		for _, hookValue := range hooks {
			hook, ok := hookValue.(map[string]any)
			command, _ := hook["command"].(string)
			if ok && hook["type"] == "command" && isDossierHookCommand(command) {
				changed = true
				continue
			}
			keptHooks = append(keptHooks, hookValue)
		}
		if len(keptHooks) == 0 {
			if len(keptHooks) != len(hooks) {
				continue
			}
		}
		if len(keptHooks) != len(hooks) {
			matcher["hooks"] = keptHooks
		}
		keptMatchers = append(keptMatchers, matcherValue)
	}
	if !changed {
		return value, false
	}
	return keptMatchers, true
}

func isDossierMCPEntry(value any) bool {
	entry, ok := value.(map[string]any)
	if !ok {
		return false
	}
	command, _ := entry["command"].(string)
	args, ok := entry["args"].([]any)
	return command != "" && ok && len(args) == 2 && args[0] == "mcp" && args[1] == "serve"
}

func removeDossierConfigMap(config map[string]any) bool {
	changed := false
	if servers, ok := config["mcpServers"].(map[string]any); ok {
		removed := false
		if entry, ok := servers["dossier"]; ok && isDossierMCPEntry(entry) {
			delete(servers, "dossier")
			removed = true
			changed = true
		}
		if removed && len(servers) == 0 {
			delete(config, "mcpServers")
		}
	}
	if hooks, ok := config["hooks"].(map[string]any); ok {
		removed := false
		for _, name := range []string{"SessionStart", "SessionEnd", "PreCompact"} {
			if cleaned, found := removeDossierHooks(hooks[name]); found {
				if arr, ok := cleaned.([]any); ok && len(arr) == 0 {
					delete(hooks, name)
				} else {
					hooks[name] = cleaned
				}
				removed = true
				changed = true
			}
		}
		if removed && len(hooks) == 0 {
			delete(config, "hooks")
		}
	}
	if values, ok := config["customInstructions"].([]any); ok {
		kept := make([]any, 0, len(values))
		removed := false
		for _, value := range values {
			if value == claudeSkillInstruction {
				removed = true
				changed = true
				continue
			}
			kept = append(kept, value)
		}
		if removed && len(kept) == 0 {
			delete(config, "customInstructions")
		} else if removed {
			config["customInstructions"] = kept
		}
	} else if value, ok := config["customInstructions"].(string); ok && value == claudeSkillInstruction {
		delete(config, "customInstructions")
		changed = true
	}
	return changed
}

func claudeConfigNeedsRemoval(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if len(data) == 0 {
		return false, nil
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return false, fmt.Errorf("parse Claude Code config %s: %w", path, err)
	}
	return removeDossierConfigMap(config), nil
}

func removeClaudeConfig(path string, timestamp int64) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parse Claude Code config %s: %w", path, err)
	}
	if !removeDossierConfigMap(config) {
		return nil
	}
	backupPath := fmt.Sprintf("%s.%d.bak", path, timestamp)
	if len(data) > 0 {
		if err := os.WriteFile(backupPath, data, 0644); err != nil {
			return fmt.Errorf("back up Claude Code config: %w", err)
		}
	}
	updated, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal Claude Code config: %w", err)
	}
	if err := os.WriteFile(path, updated, 0644); err != nil {
		return fmt.Errorf("write Claude Code config: %w", err)
	}
	return nil
}

// Uninstall removes Dossier-owned Claude Code configuration and managed skills.
// Unrelated MCP servers, hooks, instructions, and user-edited skills remain.
func (c *ClaudeCodeHarness) Uninstall(opts core.InstallOpts) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	claudeJSONPath := filepath.Join(home, ".claude.json")
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	delegatePath := filepath.Join(home, ".claude", "skills", "dossier-delegate", "SKILL.md")
	sparkPath := filepath.Join(home, ".claude", "skills", "spark", "SKILL.md")
	delegateContent, err := assets.FS.ReadFile("dossier-delegate-skill.md")
	if err != nil {
		return fmt.Errorf("failed to read embedded delegate skill asset: %w", err)
	}
	sparkContent, err := assets.FS.ReadFile("spark-skill.md")
	if err != nil {
		return fmt.Errorf("failed to read embedded spark skill asset: %w", err)
	}

	configChange := false
	for _, path := range []string{claudeJSONPath, settingsPath} {
		needs, err := claudeConfigNeedsRemoval(path)
		if err != nil {
			return err
		}
		configChange = configChange || needs
	}
	assetChange := false
	for _, item := range []struct {
		path    string
		content []byte
	}{
		{delegatePath, delegateContent},
		{sparkPath, sparkContent},
	} {
		if existing, readErr := os.ReadFile(item.path); readErr == nil {
			if !bytes.Equal(existing, item.content) {
				return fmt.Errorf("refusing to remove modified managed asset %s; remove it manually if it is no longer needed", item.path)
			}
			assetChange = true
		}
	}
	if !configChange && !assetChange {
		return nil
	}
	if !opts.YesToAll {
		stat, err := os.Stdin.Stat()
		if err != nil || (stat.Mode()&os.ModeCharDevice) == 0 {
			return core.ErrUninstallSkipped
		}
		fmt.Print("Remove the Dossier Claude Code integration? [y/N]: ")
		var response string
		_, _ = fmt.Scanln(&response)
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			return core.ErrUninstallSkipped
		}
	}
	timestamp := time.Now().Unix()
	for _, path := range []string{claudeJSONPath, settingsPath} {
		if err := removeClaudeConfig(path, timestamp); err != nil {
			return err
		}
	}
	if err := removeManagedAsset(delegatePath, delegateContent); err != nil {
		return err
	}
	return removeManagedAsset(sparkPath, sparkContent)
}

// ResolveTranscript attempts to read the transcript file either from the provided path or by finding the file named <sessionID>.jsonl under ~/.claude/projects.
func ResolveTranscript(sessionID, transcriptPath string) string {
	if transcriptPath != "" {
		if b, err := os.ReadFile(transcriptPath); err == nil {
			return string(b)
		}
	}
	if sessionID != "" {
		home, err := os.UserHomeDir()
		if err == nil {
			projectsDir := filepath.Join(home, ".claude", "projects")
			var foundPath string
			_ = filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				if !info.IsDir() && info.Name() == sessionID+".jsonl" {
					foundPath = path
					return filepath.SkipAll
				}
				return nil
			})
			if foundPath != "" {
				if b, err := os.ReadFile(foundPath); err == nil {
					return string(b)
				}
			}
		}
	}
	return ""
}
