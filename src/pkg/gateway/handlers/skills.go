// Package handlers provides skills-related Gateway method handlers.
package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/openocta/openocta/pkg/agent"
	agentSkills "github.com/openocta/openocta/pkg/agent/skills"
	"github.com/openocta/openocta/pkg/config"
	"github.com/openocta/openocta/pkg/gateway/protocol"
	"github.com/openocta/openocta/pkg/installmetadata"
	"github.com/openocta/openocta/pkg/paths"
	"gopkg.in/yaml.v3"
)

// SkillsStatusParams holds parameters for skills.status.
type SkillsStatusParams struct {
	AgentID string
}

// SkillsInstallParams holds parameters for skills.install.
type SkillsInstallParams struct {
	Name      string
	InstallID string
	TimeoutMs *int
}

// SkillsUpdateParams holds parameters for skills.update.
type SkillsUpdateParams struct {
	SkillKey string
	Enabled  *bool
	APIKey   *string
	Env      map[string]string
}

// SkillStatusConfigCheck represents a config path check result.
type SkillStatusConfigCheck struct {
	Path      string      `json:"path"`
	Value     interface{} `json:"value"`
	Satisfied bool        `json:"satisfied"`
}

// SkillInstallOption represents an installation option for a skill.
type SkillInstallOption struct {
	ID    string   `json:"id"`
	Kind  string   `json:"kind"` // "brew", "node", "go", "uv", "download"
	Label string   `json:"label"`
	Bins  []string `json:"bins"`
}

// SkillStatusRequirements holds skill requirements.
type SkillStatusRequirements struct {
	Bins    []string `json:"bins"`
	AnyBins []string `json:"anyBins"`
	Env     []string `json:"env"`
	Config  []string `json:"config"`
	OS      []string `json:"os"`
}

// SkillStatusMissing holds missing requirements.
type SkillStatusMissing struct {
	Bins    []string `json:"bins"`
	AnyBins []string `json:"anyBins"`
	Env     []string `json:"env"`
	Config  []string `json:"config"`
	OS      []string `json:"os"`
}

// SkillStatusEntry represents a skill status entry in the status report.
type SkillStatusEntry struct {
	Name               string                   `json:"name"`
	Description        string                   `json:"description"`
	Source             string                   `json:"source"`
	Bundled            bool                     `json:"bundled"`
	FilePath           string                   `json:"filePath"`
	BaseDir            string                   `json:"baseDir"`
	SkillKey           string                   `json:"skillKey"`
	PrimaryEnv         string                   `json:"primaryEnv,omitempty"`
	Emoji              string                   `json:"emoji,omitempty"`
	Homepage           string                   `json:"homepage,omitempty"`
	Always             bool                     `json:"always"`
	Disabled           bool                     `json:"disabled"`
	BlockedByAllowlist bool                     `json:"blockedByAllowlist"`
	Eligible           bool                     `json:"eligible"`
	Requirements       SkillStatusRequirements  `json:"requirements"`
	Missing            SkillStatusMissing       `json:"missing"`
	ConfigChecks       []SkillStatusConfigCheck `json:"configChecks"`
	Install            []SkillInstallOption     `json:"install"`
}

// SkillStatusReport represents the complete skill status report.
type SkillStatusReport struct {
	WorkspaceDir     string             `json:"workspaceDir"`
	ManagedSkillsDir string             `json:"managedSkillsDir"`
	Skills           []SkillStatusEntry `json:"skills"`
}

// SkillEntry represents a loaded skill entry for handlers.
type SkillEntry struct {
	Name        string
	Source      string
	FilePath    string
	BaseDir     string
	Description string
	Metadata    *SkillMetadata
}

// skillEntryMatchesClientKey is true when id equals logical name, metadata skillKey, or the on-disk folder under skills/.
func skillEntryMatchesClientKey(e SkillEntry, id string) bool {
	if strings.TrimSpace(id) == "" {
		return false
	}
	if e.Name == id {
		return true
	}
	if e.Metadata != nil && e.Metadata.SkillKey != "" && e.Metadata.SkillKey == id {
		return true
	}
	return filepath.Base(e.BaseDir) == id
}

// canonicalSkillEntryConfigKey returns the key used in config skills.entries (same rule as skills.status skillKey).
func canonicalSkillEntryConfigKey(e SkillEntry) string {
	if e.Metadata != nil && e.Metadata.SkillKey != "" {
		return e.Metadata.SkillKey
	}
	return e.Name
}

// resolveWorkspaceSkillConfigKey maps a client id (e.g. market folder) to the config entry key, or returns id if unknown.
func resolveWorkspaceSkillConfigKey(cfg *config.OpenOctaConfig, env func(string) string, clientKey string) string {
	clientKey = strings.TrimSpace(clientKey)
	if cfg == nil || clientKey == "" {
		return clientKey
	}
	for _, wsDir := range listWorkspaceDirs(cfg, env) {
		for _, e := range loadWorkspaceSkillEntries(wsDir, cfg) {
			if skillEntryMatchesClientKey(e, clientKey) {
				return canonicalSkillEntryConfigKey(e)
			}
		}
	}
	return clientKey
}

func skillAgentEntryMatchesClientKey(e agentSkills.Entry, id string) bool {
	if strings.TrimSpace(id) == "" {
		return false
	}
	if e.Name == id {
		return true
	}
	if e.Metadata != nil && e.Metadata.SkillKey != "" && e.Metadata.SkillKey == id {
		return true
	}
	return filepath.Base(e.BaseDir) == id
}

// SkillMetadata holds skill metadata.
type SkillMetadata struct {
	SkillKey   string
	PrimaryEnv string
	Always     *bool
	OS         []string
	Requires   *SkillRequires
	Install    []SkillInstallSpec
	Emoji      string
	Homepage   string
}

// SkillRequires holds skill requirements.
type SkillRequires struct {
	Bins    []string
	AnyBins []string
	Env     []string
	Config  []string
}

// SkillInstallSpec holds skill installation specification.
type SkillInstallSpec struct {
	ID      string
	Kind    string
	Bins    []string
	OS      []string
	Label   string
	Formula string
	Package string
	Module  string
	URL     string
}

// parseSkillsStatusParams parses and validates skills.status params.
func parseSkillsStatusParams(params map[string]interface{}) (*SkillsStatusParams, error) {
	p := &SkillsStatusParams{}
	if agentIDRaw, ok := params["agentId"].(string); ok {
		p.AgentID = strings.TrimSpace(agentIDRaw)
	}
	return p, nil
}

// parseSkillsInstallParams parses and validates skills.install params.
func parseSkillsInstallParams(params map[string]interface{}) (*SkillsInstallParams, error) {
	p := &SkillsInstallParams{}
	name, ok := params["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	p.Name = strings.TrimSpace(name)

	installID, ok := params["installId"].(string)
	if !ok || strings.TrimSpace(installID) == "" {
		return nil, fmt.Errorf("installId is required")
	}
	p.InstallID = strings.TrimSpace(installID)

	if timeoutMs, ok := params["timeoutMs"].(float64); ok && timeoutMs >= 1000 {
		timeoutMsInt := int(timeoutMs)
		p.TimeoutMs = &timeoutMsInt
	} else if timeoutMs, ok := params["timeoutMs"].(int); ok && timeoutMs >= 1000 {
		p.TimeoutMs = &timeoutMs
	}
	return p, nil
}

// parseSkillsUpdateParams parses and validates skills.update params.
func parseSkillsUpdateParams(params map[string]interface{}) (*SkillsUpdateParams, error) {
	p := &SkillsUpdateParams{}
	skillKey, ok := params["skillKey"].(string)
	if !ok || strings.TrimSpace(skillKey) == "" {
		return nil, fmt.Errorf("skillKey is required")
	}
	p.SkillKey = strings.TrimSpace(skillKey)

	if enabled, ok := params["enabled"].(bool); ok {
		p.Enabled = &enabled
	}

	if apiKey, ok := params["apiKey"].(string); ok {
		trimmed := strings.TrimSpace(apiKey)
		if trimmed != "" {
			p.APIKey = &trimmed
		}
	}

	if envRaw, ok := params["env"].(map[string]interface{}); ok {
		p.Env = make(map[string]string)
		for k, v := range envRaw {
			if val, ok := v.(string); ok {
				trimmedKey := strings.TrimSpace(k)
				trimmedVal := strings.TrimSpace(val)
				if trimmedKey != "" {
					if trimmedVal != "" {
						p.Env[trimmedKey] = trimmedVal
					} else {
						// Empty value means delete
						p.Env[trimmedKey] = ""
					}
				}
			}
		}
	}
	return p, nil
}

// normalizeAgentID and resolveDefaultAgentID are defined in sessions.go

// listAgentIDs lists all agent IDs from config.
func listAgentIDs(cfg *config.OpenOctaConfig) []string {
	if cfg == nil || cfg.Agents == nil || len(cfg.Agents.List) == 0 {
		return []string{"main"}
	}
	seen := make(map[string]bool)
	var ids []string
	for i := range cfg.Agents.List {
		agent := &cfg.Agents.List[i]
		id := normalizeAgentID(agent.ID)
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return []string{"main"}
	}
	return ids
}

// resolveAgentWorkspaceDir resolves workspace directory for an agent (delegates to agent package).
func resolveAgentWorkspaceDir(cfg *config.OpenOctaConfig, agentID string, env func(string) string) string {
	return agent.ResolveAgentWorkspaceDir(cfg, agentID, env)
}

// listWorkspaceDirs lists all workspace directories from config.
func listWorkspaceDirs(cfg *config.OpenOctaConfig, env func(string) string) []string {
	dirs := make(map[string]bool)
	agentIDs := listAgentIDs(cfg)
	for _, agentID := range agentIDs {
		dir := resolveAgentWorkspaceDir(cfg, agentID, env)
		dirs[dir] = true
	}
	// Also add default agent workspace
	defaultAgentID := resolveDefaultAgentID(cfg)
	defaultDir := resolveAgentWorkspaceDir(cfg, defaultAgentID, env)
	dirs[defaultDir] = true

	var result []string
	for dir := range dirs {
		result = append(result, dir)
	}
	return result
}

// loadWorkspaceSkillEntries loads skill entries from a workspace directory.
func loadWorkspaceSkillEntries(workspaceDir string, cfg *config.OpenOctaConfig) []SkillEntry {
	opts := &agentSkills.LoadOptions{
		Config:           cfg,
		ManagedSkillsDir: "",
		BundledSkillsDir: "",
	}

	entries, err := agentSkills.LoadWorkspaceEntries(workspaceDir, opts)
	if err != nil {
		// Log error but return empty list
		return []SkillEntry{}
	}

	// Convert to handler SkillEntry format
	var result []SkillEntry
	for _, entry := range entries {
		// Extract description from frontmatter
		description := entry.Name
		if desc, ok := entry.Frontmatter["description"]; ok && desc != "" {
			description = desc
		}

		skillEntry := SkillEntry{
			Name:        entry.Name,
			Source:      entry.Source,
			FilePath:    entry.FilePath,
			BaseDir:     entry.BaseDir,
			Description: description,
		}

		if entry.Metadata != nil {
			skillEntry.Metadata = &SkillMetadata{
				SkillKey:   entry.Metadata.SkillKey,
				PrimaryEnv: entry.Metadata.PrimaryEnv,
				Always:     entry.Metadata.Always,
				OS:         entry.Metadata.OS,
				Emoji:      entry.Metadata.Emoji,
				Homepage:   entry.Metadata.Homepage,
			}

			if entry.Metadata.Requires != nil {
				skillEntry.Metadata.Requires = &SkillRequires{
					Bins:    entry.Metadata.Requires.Bins,
					AnyBins: entry.Metadata.Requires.AnyBins,
					Env:     entry.Metadata.Requires.Env,
					Config:  entry.Metadata.Requires.Config,
				}
			}

			if len(entry.Metadata.Install) > 0 {
				skillEntry.Metadata.Install = make([]SkillInstallSpec, len(entry.Metadata.Install))
				for i, spec := range entry.Metadata.Install {
					skillEntry.Metadata.Install[i] = SkillInstallSpec{
						ID:      spec.ID,
						Kind:    spec.Kind,
						Bins:    spec.Bins,
						OS:      spec.OS,
						Label:   spec.Label,
						Formula: spec.Formula,
						Package: spec.Package,
						Module:  spec.Module,
						URL:     spec.URL,
					}
				}
			}
		}

		result = append(result, skillEntry)
	}

	return result
}

// collectSkillBins collects all required bins from skill entries.
func collectSkillBins(entries []SkillEntry) []string {
	bins := make(map[string]bool)
	for _, entry := range entries {
		if entry.Metadata == nil {
			continue
		}
		if entry.Metadata.Requires != nil {
			for _, bin := range entry.Metadata.Requires.Bins {
				trimmed := strings.TrimSpace(bin)
				if trimmed != "" {
					bins[trimmed] = true
				}
			}
			for _, bin := range entry.Metadata.Requires.AnyBins {
				trimmed := strings.TrimSpace(bin)
				if trimmed != "" {
					bins[trimmed] = true
				}
			}
		}
		for _, spec := range entry.Metadata.Install {
			for _, bin := range spec.Bins {
				trimmed := strings.TrimSpace(bin)
				if trimmed != "" {
					bins[trimmed] = true
				}
			}
		}
	}
	var result []string
	for bin := range bins {
		result = append(result, bin)
	}
	sort.Strings(result)
	return result
}

// hasBinary checks if a binary exists in PATH.
func hasBinary(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

// resolveSkillConfig resolves skill configuration from config.
func resolveSkillConfig(cfg *config.OpenOctaConfig, skillKey string) *config.SkillConfig {
	if cfg == nil || cfg.Skills == nil || cfg.Skills.Entries == nil {
		return nil
	}
	entry, ok := cfg.Skills.Entries[skillKey]
	if !ok {
		return nil
	}
	return &entry
}

// resolveBundledAllowlist resolves the bundled skills allowlist from config.
func resolveBundledAllowlist(cfg *config.OpenOctaConfig) []string {
	if cfg == nil || cfg.Skills == nil || len(cfg.Skills.AllowBundled) == 0 {
		return nil
	}
	var result []string
	for _, name := range cfg.Skills.AllowBundled {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// isBundledSkillAllowed checks if a bundled skill is allowed based on allowlist.
func isBundledSkillAllowed(skillName string, skillKey string, source string, allowlist []string) bool {
	if len(allowlist) == 0 {
		return true
	}
	// Only check bundled skills
	if source != "openclaw-bundled" {
		return true
	}
	// Check if skill name or key is in allowlist
	for _, allowed := range allowlist {
		if allowed == skillName || allowed == skillKey {
			return true
		}
	}
	return false
}

// resolveConfigPath resolves a config path value (e.g., "browser.enabled").
func resolveConfigPath(cfg *config.OpenOctaConfig, pathStr string) interface{} {
	if cfg == nil || pathStr == "" {
		return nil
	}
	parts := strings.Split(pathStr, ".")
	var current interface{} = cfg
	for _, part := range parts {
		if part == "" {
			continue
		}
		// Use reflection or type assertion to navigate config
		// For now, handle common cases
		switch part {
		case "browser":
			if cfg.Browser != nil {
				current = cfg.Browser
			} else {
				return nil
			}
		case "enabled":
			if browser, ok := current.(*config.BrowserConfig); ok && browser.Enabled != nil {
				return *browser.Enabled
			}
			return nil
		case "evaluateEnabled":
			if browser, ok := current.(*config.BrowserConfig); ok && browser.EvaluateEnabled != nil {
				return *browser.EvaluateEnabled
			}
			return nil
		default:
			// For other paths, return nil (can be extended later)
			return nil
		}
	}
	return current
}

// isConfigPathTruthy checks if a config path value is truthy.
func isConfigPathTruthy(cfg *config.OpenOctaConfig, pathStr string) bool {
	value := resolveConfigPath(cfg, pathStr)
	if value == nil {
		// Check default values
		defaultValues := map[string]bool{
			"browser.enabled":         true,
			"browser.evaluateEnabled": true,
		}
		if defaultValue, ok := defaultValues[pathStr]; ok {
			return defaultValue
		}
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case int:
		return v != 0
	case string:
		return strings.TrimSpace(v) != ""
	default:
		return true
	}
}

// resolveSkillsInstallPreferences resolves skill installation preferences from config.
func resolveSkillsInstallPreferences(cfg *config.OpenOctaConfig) (preferBrew bool, nodeManager string) {
	preferBrew = true
	nodeManager = "npm"

	if cfg == nil || cfg.Skills == nil || cfg.Skills.Install == nil {
		return preferBrew, nodeManager
	}

	if cfg.Skills.Install.PreferBrew != nil {
		preferBrew = *cfg.Skills.Install.PreferBrew
	}

	if cfg.Skills.Install.NodeManager != nil {
		manager := strings.ToLower(strings.TrimSpace(*cfg.Skills.Install.NodeManager))
		switch manager {
		case "pnpm", "yarn", "bun", "npm":
			nodeManager = manager
		default:
			nodeManager = "npm"
		}
	}

	return preferBrew, nodeManager
}

// resolveRuntimePlatform returns the current runtime platform.
func resolveRuntimePlatform() string {
	return runtime.GOOS
}

// ResolveManagedSkillsDir resolves the managed skills directory path (~/.openocta/skills).
func ResolveManagedSkillsDir(env func(string) string) string {
	stateDir := paths.ResolveStateDir(env)
	return filepath.Join(stateDir, "skills")
}

// selectPreferredInstallSpec selects the preferred install spec based on preferences.
func selectPreferredInstallSpec(install []SkillInstallSpec, preferBrew bool, nodeManager string) *SkillInstallSpec {
	if len(install) == 0 {
		return nil
	}

	// Find specs by kind
	var brewSpec, nodeSpec, goSpec, uvSpec *SkillInstallSpec
	for i := range install {
		spec := &install[i]
		switch spec.Kind {
		case "brew":
			if brewSpec == nil {
				brewSpec = spec
			}
		case "node":
			if nodeSpec == nil {
				nodeSpec = spec
			}
		case "go":
			if goSpec == nil {
				goSpec = spec
			}
		case "uv":
			if uvSpec == nil {
				uvSpec = spec
			}
		}
	}

	// Select based on preferences
	if preferBrew && hasBinary("brew") && brewSpec != nil {
		return brewSpec
	}
	if uvSpec != nil {
		return uvSpec
	}
	if nodeSpec != nil {
		return nodeSpec
	}
	if brewSpec != nil {
		return brewSpec
	}
	if goSpec != nil {
		return goSpec
	}

	// Return first available
	return &install[0]
}

// normalizeInstallOptions normalizes install options for a skill entry.
func normalizeInstallOptions(entry SkillEntry, preferBrew bool, nodeManager string) []SkillInstallOption {
	if entry.Metadata == nil || len(entry.Metadata.Install) == 0 {
		return []SkillInstallOption{}
	}

	install := entry.Metadata.Install
	currentPlatform := resolveRuntimePlatform()

	// Filter by OS
	filtered := []SkillInstallSpec{}
	for _, spec := range install {
		if len(spec.OS) == 0 {
			filtered = append(filtered, spec)
		} else {
			for _, osName := range spec.OS {
				if osName == currentPlatform {
					filtered = append(filtered, spec)
					break
				}
			}
		}
	}

	if len(filtered) == 0 {
		return []SkillInstallOption{}
	}

	// Check if all are downloads
	allDownloads := true
	for _, spec := range filtered {
		if spec.Kind != "download" {
			allDownloads = false
			break
		}
	}

	if allDownloads {
		// Return all download options
		options := []SkillInstallOption{}
		for i, spec := range filtered {
			options = append(options, buildInstallOption(&spec, i, nodeManager))
		}
		return options
	}

	// Select preferred spec
	preferred := selectPreferredInstallSpec(filtered, preferBrew, nodeManager)
	if preferred == nil {
		return []SkillInstallOption{}
	}

	// Find index
	index := 0
	for i, spec := range filtered {
		if spec.ID == preferred.ID && spec.Kind == preferred.Kind {
			index = i
			break
		}
	}

	return []SkillInstallOption{buildInstallOption(preferred, index, nodeManager)}
}

// buildInstallOption builds a SkillInstallOption from an InstallSpec.
func buildInstallOption(spec *SkillInstallSpec, index int, nodeManager string) SkillInstallOption {
	id := spec.ID
	if id == "" {
		id = fmt.Sprintf("%s-%d", spec.Kind, index)
	}
	id = strings.TrimSpace(id)

	bins := spec.Bins
	if bins == nil {
		bins = []string{}
	}

	label := strings.TrimSpace(spec.Label)
	if label == "" {
		switch spec.Kind {
		case "brew":
			if spec.Formula != "" {
				label = fmt.Sprintf("Install %s (brew)", spec.Formula)
			} else {
				label = "Run installer"
			}
		case "node":
			if spec.Package != "" {
				label = fmt.Sprintf("Install %s (%s)", spec.Package, nodeManager)
			} else {
				label = "Run installer"
			}
		case "go":
			if spec.Module != "" {
				label = fmt.Sprintf("Install %s (go)", spec.Module)
			} else {
				label = "Run installer"
			}
		case "uv":
			if spec.Package != "" {
				label = fmt.Sprintf("Install %s (uv)", spec.Package)
			} else {
				label = "Run installer"
			}
		case "download":
			if spec.URL != "" {
				url := strings.TrimSpace(spec.URL)
				parts := strings.Split(url, "/")
				last := ""
				if len(parts) > 0 {
					last = parts[len(parts)-1]
				}
				if last != "" {
					label = fmt.Sprintf("Download %s", last)
				} else {
					label = fmt.Sprintf("Download %s", url)
				}
			} else {
				label = "Run installer"
			}
		default:
			label = "Run installer"
		}
	}

	return SkillInstallOption{
		ID:    id,
		Kind:  spec.Kind,
		Label: label,
		Bins:  bins,
	}
}

// buildSkillStatus builds a skill status entry from a skill entry.
func buildSkillStatus(entry SkillEntry, cfg *config.OpenOctaConfig, preferBrew bool, nodeManager string) SkillStatusEntry {
	skillKey := entry.Name
	if entry.Metadata != nil && entry.Metadata.SkillKey != "" {
		skillKey = entry.Metadata.SkillKey
	}

	skillConfig := resolveSkillConfig(cfg, skillKey)
	disabled := skillConfig != nil && skillConfig.Enabled != nil && !*skillConfig.Enabled
	allowBundled := resolveBundledAllowlist(cfg)
	source := entry.Source
	if source == "" {
		source = "unknown"
	}
	blockedByAllowlist := !isBundledSkillAllowed(entry.Name, skillKey, source, allowBundled)

	// Use description from entry (already extracted from frontmatter)
	description := entry.Description
	if description == "" {
		description = entry.Name
	}

	// Extract emoji and homepage
	emoji := ""
	homepage := ""
	if entry.Metadata != nil {
		emoji = entry.Metadata.Emoji
		homepage = entry.Metadata.Homepage
	}

	// Check if bundled
	bundled := source == "openclaw-bundled"

	// Check requirements
	// Initialize all slices as empty arrays, not nil
	missingBins := []string{}
	missingAnyBins := []string{}
	missingEnv := []string{}
	missingConfig := []string{}
	missingOs := []string{}
	requiredBins := []string{}
	requiredAnyBins := []string{}
	requiredEnv := []string{}
	requiredConfig := []string{}
	requiredOs := []string{}

	if entry.Metadata != nil && entry.Metadata.Requires != nil {
		if entry.Metadata.Requires.Bins != nil {
			requiredBins = entry.Metadata.Requires.Bins
		}
		if entry.Metadata.Requires.AnyBins != nil {
			requiredAnyBins = entry.Metadata.Requires.AnyBins
		}
		if entry.Metadata.Requires.Env != nil {
			requiredEnv = entry.Metadata.Requires.Env
		}
		if entry.Metadata.Requires.Config != nil {
			requiredConfig = entry.Metadata.Requires.Config
		}

		// Check bins
		for _, bin := range requiredBins {
			if !hasBinary(bin) {
				missingBins = append(missingBins, bin)
			}
		}
		// Check anyBins
		if len(requiredAnyBins) > 0 {
			anyFound := false
			for _, bin := range requiredAnyBins {
				if hasBinary(bin) {
					anyFound = true
					break
				}
			}
			if !anyFound {
				missingAnyBins = requiredAnyBins
			}
		}
		// Check env
		for _, envName := range requiredEnv {
			if os.Getenv(envName) == "" {
				if skillConfig == nil || skillConfig.Env == nil || skillConfig.Env[envName] == "" {
					if skillConfig == nil || skillConfig.APIKey == "" || entry.Metadata.PrimaryEnv != envName {
						missingEnv = append(missingEnv, envName)
					}
				}
			}
		}
		// Check config
		for _, configPath := range requiredConfig {
			if !isConfigPathTruthy(cfg, configPath) {
				missingConfig = append(missingConfig, configPath)
			}
		}
	}

	// Check OS
	if entry.Metadata != nil {
		requiredOs = entry.Metadata.OS
		if len(requiredOs) > 0 {
			currentPlatform := resolveRuntimePlatform()
			found := false
			for _, osName := range requiredOs {
				if osName == currentPlatform {
					found = true
					break
				}
			}
			if !found {
				missingOs = requiredOs
			}
		}
	}

	// Build config checks
	configChecks := []SkillStatusConfigCheck{}
	for _, configPath := range requiredConfig {
		value := resolveConfigPath(cfg, configPath)
		satisfied := isConfigPathTruthy(cfg, configPath)
		configChecks = append(configChecks, SkillStatusConfigCheck{
			Path:      configPath,
			Value:     value,
			Satisfied: satisfied,
		})
	}

	always := entry.Metadata != nil && entry.Metadata.Always != nil && *entry.Metadata.Always

	// If always is true, missing should be empty
	if always {
		missingBins = []string{}
		missingAnyBins = []string{}
		missingEnv = []string{}
		missingConfig = []string{}
		missingOs = []string{}
	}

	eligible := !disabled && !blockedByAllowlist && (always ||
		(len(missingBins) == 0 && len(missingAnyBins) == 0 && len(missingEnv) == 0 && len(missingConfig) == 0 && len(missingOs) == 0))

	// Build install options
	installOptions := normalizeInstallOptions(entry, preferBrew, nodeManager)

	primaryEnv := ""
	if entry.Metadata != nil {
		primaryEnv = entry.Metadata.PrimaryEnv
	}

	return SkillStatusEntry{
		Name:               entry.Name,
		Description:        description,
		Source:             source,
		Bundled:            bundled,
		FilePath:           entry.FilePath,
		BaseDir:            entry.BaseDir,
		SkillKey:           skillKey,
		PrimaryEnv:         primaryEnv,
		Emoji:              emoji,
		Homepage:           homepage,
		Always:             always,
		Disabled:           disabled,
		BlockedByAllowlist: blockedByAllowlist,
		Eligible:           eligible,
		Requirements: SkillStatusRequirements{
			Bins:    requiredBins,
			AnyBins: requiredAnyBins,
			Env:     requiredEnv,
			Config:  requiredConfig,
			OS:      requiredOs,
		},
		Missing: SkillStatusMissing{
			Bins:    missingBins,
			AnyBins: missingAnyBins,
			Env:     missingEnv,
			Config:  missingConfig,
			OS:      missingOs,
		},
		ConfigChecks: configChecks,
		Install:      installOptions,
	}
}

// buildWorkspaceSkillStatus builds a skill status report for a workspace.
func buildWorkspaceSkillStatus(workspaceDir string, cfg *config.OpenOctaConfig, env func(string) string) SkillStatusReport {
	managedSkillsDir := ResolveManagedSkillsDir(env)
	entries := loadWorkspaceSkillEntries(workspaceDir, cfg)

	preferBrew, nodeManager := resolveSkillsInstallPreferences(cfg)

	// Build skill status entries
	skillStatuses := []SkillStatusEntry{}
	for _, entry := range entries {
		status := buildSkillStatus(entry, cfg, preferBrew, nodeManager)
		skillStatuses = append(skillStatuses, status)
	}
	sort.Slice(skillStatuses, func(i, j int) bool {
		ki, kj := skillStatuses[i].SkillKey, skillStatuses[j].SkillKey
		if ki == "" {
			ki = skillStatuses[i].Name
		}
		if kj == "" {
			kj = skillStatuses[j].Name
		}
		return strings.Compare(strings.ToLower(ki), strings.ToLower(kj)) < 0
	})

	return SkillStatusReport{
		WorkspaceDir:     workspaceDir,
		ManagedSkillsDir: managedSkillsDir,
		Skills:           skillStatuses,
	}
}

// SkillsStatusHandler handles "skills.status".
func SkillsStatusHandler(opts HandlerOpts) error {
	params, err := parseSkillsStatusParams(opts.Params)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInvalidRequest,
			Message: fmt.Sprintf("invalid skills.status params: %v", err),
		}, nil)
		return nil
	}

	// Load config from disk so skills.entries (including enabled) is always current
	env := func(k string) string { return os.Getenv(k) }
	cfg, err := config.Load(env)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInternal,
			Message: "failed to load config: " + err.Error(),
		}, nil)
		return nil
	}

	// Resolve agent ID
	agentIDRaw := params.AgentID
	agentID := agentIDRaw
	if agentID == "" {
		agentID = resolveDefaultAgentID(cfg)
	} else {
		agentID = normalizeAgentID(agentID)
		// Validate agent ID
		knownAgents := listAgentIDs(cfg)
		found := false
		for _, knownID := range knownAgents {
			if knownID == agentID {
				found = true
				break
			}
		}
		if !found && agentIDRaw != "" {
			opts.Respond(false, nil, &protocol.ErrorShape{
				Code:    protocol.ErrCodeInvalidRequest,
				Message: fmt.Sprintf("unknown agent id \"%s\"", agentIDRaw),
			}, nil)
			return nil
		}
	}

	// Resolve workspace directory
	workspaceDir := resolveAgentWorkspaceDir(cfg, agentID, env)

	// Build status report
	report := buildWorkspaceSkillStatus(workspaceDir, cfg, env)

	opts.Respond(true, report, nil, nil)
	return nil
}

// SkillsGetDocParams holds parameters for skills.getDoc.
type SkillsGetDocParams struct {
	SkillKey string
	AgentID  string
}

func parseSkillsGetDocParams(params map[string]interface{}) (*SkillsGetDocParams, error) {
	p := &SkillsGetDocParams{}
	skillKey, ok := params["skillKey"].(string)
	if !ok || strings.TrimSpace(skillKey) == "" {
		return nil, fmt.Errorf("skillKey is required")
	}
	p.SkillKey = strings.TrimSpace(skillKey)
	if agentID, ok := params["agentId"].(string); ok {
		p.AgentID = strings.TrimSpace(agentID)
	}
	return p, nil
}

// SkillsGetDocHandler handles "skills.getDoc". Returns the SKILL.md content for the given skill
// (from disk or embedded). Used by the UI to show skill documentation in the detail modal.
func SkillsGetDocHandler(opts HandlerOpts) error {
	params, err := parseSkillsGetDocParams(opts.Params)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInvalidRequest,
			Message: fmt.Sprintf("invalid skills.getDoc params: %v", err),
		}, nil)
		return nil
	}

	env := func(k string) string { return os.Getenv(k) }
	cfg, err := config.Load(env)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInternal,
			Message: "failed to load config: " + err.Error(),
		}, nil)
		return nil
	}

	agentID := params.AgentID
	if agentID == "" {
		agentID = resolveDefaultAgentID(cfg)
	} else {
		agentID = normalizeAgentID(agentID)
		knownAgents := listAgentIDs(cfg)
		found := false
		for _, id := range knownAgents {
			if id == agentID {
				found = true
				break
			}
		}
		if !found && params.AgentID != "" {
			opts.Respond(false, nil, &protocol.ErrorShape{
				Code:    protocol.ErrCodeInvalidRequest,
				Message: fmt.Sprintf("unknown agent id \"%s\"", params.AgentID),
			}, nil)
			return nil
		}
	}

	workspaceDir := resolveAgentWorkspaceDir(cfg, agentID, env)
	optsLoader := &agentSkills.LoadOptions{
		Config:           cfg,
		ManagedSkillsDir: "",
		BundledSkillsDir: "",
	}
	entries, err := agentSkills.LoadWorkspaceEntries(workspaceDir, optsLoader)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInternal,
			Message: "failed to load skills: " + err.Error(),
		}, nil)
		return nil
	}

	skillKey := params.SkillKey
	var content []byte
	for _, e := range entries {
		if !skillAgentEntryMatchesClientKey(e, skillKey) {
			continue
		}
		if len(e.EmbeddedContent) > 0 {
			content = e.EmbeddedContent
			break
		}
		content, err = os.ReadFile(e.FilePath)
		if err != nil {
			opts.Respond(false, nil, &protocol.ErrorShape{
				Code:    protocol.ErrCodeInternal,
				Message: "failed to read skill file: " + err.Error(),
			}, nil)
			return nil
		}
		break
	}

	if content == nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInvalidRequest,
			Message: fmt.Sprintf("skill not found: %s", skillKey),
		}, nil)
		return nil
	}

	opts.Respond(true, map[string]interface{}{
		"content": string(content),
	}, nil, nil)
	return nil
}

// SkillsBinsHandler handles "skills.bins".
func SkillsBinsHandler(opts HandlerOpts) error {
	// Load config
	var cfg *config.OpenOctaConfig
	if opts.Context != nil && opts.Context.Config != nil {
		cfg = opts.Context.Config
	} else {
		env := func(k string) string { return os.Getenv(k) }
		loaded, err := config.Load(env)
		if err != nil {
			opts.Respond(false, nil, &protocol.ErrorShape{
				Code:    protocol.ErrCodeInternal,
				Message: "failed to load config: " + err.Error(),
			}, nil)
			return nil
		}
		cfg = loaded
	}

	// List all workspace directories
	env := func(k string) string { return os.Getenv(k) }
	workspaceDirs := listWorkspaceDirs(cfg, env)

	// Collect bins from all workspaces
	bins := make(map[string]bool)
	for _, workspaceDir := range workspaceDirs {
		entries := loadWorkspaceSkillEntries(workspaceDir, cfg)
		for _, bin := range collectSkillBins(entries) {
			bins[bin] = true
		}
	}

	// Convert to sorted slice
	var result []string
	for bin := range bins {
		result = append(result, bin)
	}
	sort.Strings(result)

	opts.Respond(true, map[string]interface{}{
		"bins": result,
	}, nil, nil)
	return nil
}

// SkillsInstallHandler handles "skills.install" (stub: not implemented).
func SkillsInstallHandler(opts HandlerOpts) error {
	_, err := parseSkillsInstallParams(opts.Params)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInvalidRequest,
			Message: fmt.Sprintf("invalid skills.install params: %v", err),
		}, nil)
		return nil
	}

	// Stub: return not implemented error
	opts.Respond(false, nil, &protocol.ErrorShape{
		Code:    "method_not_implemented",
		Message: "skills.install not implemented",
	}, nil)
	return nil

	// TODO: Implement full installation logic:
	// 1. Load config
	// 2. Resolve default agent workspace directory
	// 3. Call installSkill equivalent
	// 4. Return result
}

// SkillsUpdateHandler handles "skills.update".
func SkillsUpdateHandler(opts HandlerOpts) error {
	params, err := parseSkillsUpdateParams(opts.Params)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInvalidRequest,
			Message: fmt.Sprintf("invalid skills.update params: %v", err),
		}, nil)
		return nil
	}

	env := func(k string) string { return os.Getenv(k) }
	snap, err := LoadConfigSnapshot(env)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInternal,
			Message: "failed to load config: " + err.Error(),
		}, nil)
		return nil
	}

	resolvedSkillKey := resolveWorkspaceSkillConfigKey(snap.Config, env, params.SkillKey)

	// Use raw file map to preserve all keys; patch only skills.entries[skillKey]
	cfgMap := ConfigSnapshotToMap(snap)
	if cfgMap == nil {
		cfgMap = map[string]interface{}{}
	}
	skillsMap, _ := cfgMap["skills"].(map[string]interface{})
	if skillsMap == nil {
		skillsMap = map[string]interface{}{}
		cfgMap["skills"] = skillsMap
	}
	entriesMap, _ := skillsMap["entries"].(map[string]interface{})
	if entriesMap == nil {
		entriesMap = map[string]interface{}{}
		skillsMap["entries"] = entriesMap
	}
	current, _ := entriesMap[resolvedSkillKey].(map[string]interface{})
	if current == nil {
		current = map[string]interface{}{}
	}

	// Apply updates to current entry map
	if params.Enabled != nil {
		current["enabled"] = *params.Enabled
	}
	if params.APIKey != nil {
		current["apiKey"] = *params.APIKey
	}
	if params.Env != nil {
		envMap, _ := current["env"].(map[string]interface{})
		if envMap == nil {
			envMap = map[string]interface{}{}
		}
		for k, v := range params.Env {
			if v == "" {
				delete(envMap, k)
			} else {
				envMap[k] = v
			}
		}
		current["env"] = envMap
	}
	entriesMap[resolvedSkillKey] = current

	if err := WriteConfigMap(snap.Path, cfgMap); err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInternal,
			Message: "failed to write config: " + err.Error(),
		}, nil)
		return nil
	}

	// Build response
	result := map[string]interface{}{
		"ok":       true,
		"skillKey": resolvedSkillKey,
		"config": map[string]interface{}{
			"enabled": current["enabled"],
			"apiKey":  current["apiKey"],
			"env":     current["env"],
		},
	}

	opts.Respond(true, result, nil, nil)
	return nil
}

// SkillsDeleteParams holds parameters for skills.delete.
type SkillsDeleteParams struct {
	SkillKey string
}

// parseSkillsDeleteParams parses and validates skills.delete params.
func parseSkillsDeleteParams(params map[string]interface{}) (*SkillsDeleteParams, error) {
	p := &SkillsDeleteParams{}
	skillKey, ok := params["skillKey"].(string)
	if !ok || strings.TrimSpace(skillKey) == "" {
		return nil, fmt.Errorf("skillKey is required")
	}
	p.SkillKey = strings.TrimSpace(skillKey)
	return p, nil
}

// SkillsDeleteHandler handles "skills.delete". Only allows deleting skills with source
// openclaw-managed or openclaw-extra.
func SkillsDeleteHandler(opts HandlerOpts) error {
	params, err := parseSkillsDeleteParams(opts.Params)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInvalidRequest,
			Message: fmt.Sprintf("invalid skills.delete params: %v", err),
		}, nil)
		return nil
	}

	var cfg *config.OpenOctaConfig
	if opts.Context != nil && opts.Context.Config != nil {
		cfg = opts.Context.Config
	} else {
		env := func(k string) string { return os.Getenv(k) }
		loaded, err := config.Load(env)
		if err != nil {
			opts.Respond(false, nil, &protocol.ErrorShape{
				Code:    protocol.ErrCodeInternal,
				Message: "failed to load config: " + err.Error(),
			}, nil)
			return nil
		}
		cfg = loaded
	}

	env := func(k string) string { return os.Getenv(k) }
	workspaceDir := agent.ResolveAgentWorkspaceDir(cfg, resolveDefaultAgentID(cfg), env)
	entries := loadWorkspaceSkillEntries(workspaceDir, cfg)

	var targetEntry *SkillEntry
	for i := range entries {
		e := &entries[i]
		if skillEntryMatchesClientKey(*e, params.SkillKey) {
			targetEntry = e
			break
		}
	}

	if targetEntry == nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInvalidRequest,
			Message: fmt.Sprintf("skill not found: %s", params.SkillKey),
		}, nil)
		return nil
	}

	source := targetEntry.Source
	if source != "openclaw-managed" && source != "openclaw-extra" {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInvalidRequest,
			Message: fmt.Sprintf("cannot delete skill with source %s (only openclaw-managed and openclaw-extra are allowed)", source),
		}, nil)
		return nil
	}

	baseDir, err := filepath.Abs(targetEntry.BaseDir)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInternal,
			Message: "invalid baseDir: " + err.Error(),
		}, nil)
		return nil
	}

	// isSubdirOf checks if child is under parent (Windows-compatible: uses filepath.Rel).
	// On Windows, HasPrefix can fail with different drive letters or casing; Rel is reliable.
	isSubdirOf := func(parent, child string) bool {
		if parent == "" || child == "" {
			return false
		}
		rel, err := filepath.Rel(parent, child)
		if err != nil {
			return false
		}
		rel = filepath.Clean(rel)
		return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}

	managedDir, _ := filepath.Abs(ResolveManagedSkillsDir(env))
	var allowed bool
	if source == "openclaw-managed" {
		allowed = isSubdirOf(managedDir, baseDir)
	} else {
		// openclaw-extra: check baseDir is under one of extraDirs
		if cfg != nil && cfg.Skills != nil && cfg.Skills.Load != nil {
			for _, dir := range cfg.Skills.Load.ExtraDirs {
				trimmed := strings.TrimSpace(dir)
				if trimmed == "" {
					continue
				}
				absExtra := agentSkills.ResolveUserPath(trimmed, env)
				absExtra, _ = filepath.Abs(absExtra)
				if isSubdirOf(absExtra, baseDir) {
					allowed = true
					break
				}
			}
		}
	}

	if !allowed {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInvalidRequest,
			Message: "skill directory is not in an allowed location",
		}, nil)
		return nil
	}

	// Verify SKILL.md exists (safety check)
	skillFile := filepath.Join(baseDir, "SKILL.md")
	if _, err := os.Stat(skillFile); err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInvalidRequest,
			Message: "SKILL.md not found in skill directory",
		}, nil)
		return nil
	}

	if err := os.RemoveAll(baseDir); err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInternal,
			Message: "failed to delete skill: " + err.Error(),
		}, nil)
		return nil
	}
	_ = installmetadata.RemoveByRemoteID(env, "skill", params.SkillKey)
	_ = installmetadata.RemoveByLocalID(env, "skill", filepath.Base(baseDir))

	opts.Respond(true, map[string]interface{}{
		"ok":       true,
		"skillKey": params.SkillKey,
	}, nil, nil)
	return nil
}

// ========== Skill File Editing ==========

var (
	// editableExtensions defines allowed file extensions for skill editing.
	editableExtensions = map[string]bool{
		".yaml": true, ".yml": true, ".json": true, ".py": true,
		".sh": true, ".md": true, ".js": true, ".ts": true,
		".toml": true, ".txt": true, ".cfg": true, ".ini": true,
		".go": true, ".rs": true, ".css": true, ".html": true,
	}
	// blockedDirNames defines directory names that cannot be traversed or edited.
	blockedDirNames = map[string]bool{
		"node_modules": true, "dist": true, "build": true, "vendor": true,
		".git": true, "__pycache__": true, ".venv": true, "venv": true,
		".idea": true, ".vscode": true, "out": true, "target": true,
	}
)

func isEditableFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return editableExtensions[ext]
}

func isBlockedDir(name string) bool {
	return blockedDirNames[name]
}

// resolveSkillBaseDir finds a skill's base directory by skillKey across all workspaces.
func resolveSkillBaseDir(cfg *config.OpenOctaConfig, skillKey string, env func(string) string) (string, error) {
	workspaceDirs := listWorkspaceDirs(cfg, env)
	for _, workspaceDir := range workspaceDirs {
		entries := loadWorkspaceSkillEntries(workspaceDir, cfg)
		for _, entry := range entries {
			if skillEntryMatchesClientKey(entry, skillKey) {
				return entry.BaseDir, nil
			}
		}
	}
	return "", fmt.Errorf("skill not found: %s", skillKey)
}

// isSubpath checks if target is under base (both should be clean absolute paths).
func isSubpath(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	rel = filepath.Clean(rel)
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// sanitizeSkillFilePath validates and resolves a relative file path within a skill directory.
func sanitizeSkillFilePath(baseDir, relPath string) (string, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("invalid baseDir: %w", err)
	}
	// Clean the relative path to prevent traversal attacks
	cleanRel := filepath.Clean("/" + relPath)
	cleanRel = strings.TrimPrefix(cleanRel, string(filepath.Separator))
	if cleanRel == "" || cleanRel == "." {
		return "", fmt.Errorf("invalid file path")
	}
	// Check for blocked directories in the path
	parts := strings.Split(cleanRel, string(filepath.Separator))
	for _, part := range parts {
		if isBlockedDir(part) {
			return "", fmt.Errorf("access to directory %s is not allowed", part)
		}
	}
	// Ensure the file has an allowed extension
	if !isEditableFile(filepath.Base(cleanRel)) {
		return "", fmt.Errorf("file type not allowed for editing")
	}
	absTarget := filepath.Join(absBase, cleanRel)
	if !isSubpath(absBase, absTarget) {
		return "", fmt.Errorf("file path escapes skill directory")
	}
	return absTarget, nil
}

// listEditableSkillFiles recursively lists editable files under baseDir.
func listEditableSkillFiles(baseDir string) ([]string, error) {
	var files []string
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, err
	}
	err = filepath.Walk(absBase, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible
		}
		if info.IsDir() {
			if isBlockedDir(filepath.Base(path)) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isEditableFile(filepath.Base(path)) {
			return nil
		}
		rel, err := filepath.Rel(absBase, path)
		if err != nil {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	return files, err
}

// validateFileSyntax performs basic syntax validation based on file extension.
func validateFileSyntax(path string, content []byte) error {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		var v interface{}
		if err := json.Unmarshal(content, &v); err != nil {
			return fmt.Errorf("JSON syntax error: %w", err)
		}
	case ".yaml", ".yml":
		var v interface{}
		if err := yaml.Unmarshal(content, &v); err != nil {
			return fmt.Errorf("YAML syntax error: %w", err)
		}
	}
	return nil
}

// SkillsListFilesParams holds parameters for skills.listFiles.
type SkillsListFilesParams struct {
	SkillKey string
}

func parseSkillsListFilesParams(params map[string]interface{}) (*SkillsListFilesParams, error) {
	p := &SkillsListFilesParams{}
	skillKey, ok := params["skillKey"].(string)
	if !ok || strings.TrimSpace(skillKey) == "" {
		return nil, fmt.Errorf("skillKey is required")
	}
	p.SkillKey = strings.TrimSpace(skillKey)
	return p, nil
}

// SkillsListFilesHandler handles "skills.listFiles".
func SkillsListFilesHandler(opts HandlerOpts) error {
	params, err := parseSkillsListFilesParams(opts.Params)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInvalidRequest,
			Message: fmt.Sprintf("invalid skills.listFiles params: %v", err),
		}, nil)
		return nil
	}

	env := func(k string) string { return os.Getenv(k) }
	cfg, err := config.Load(env)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInternal,
			Message: "failed to load config: " + err.Error(),
		}, nil)
		return nil
	}

	baseDir, err := resolveSkillBaseDir(cfg, params.SkillKey, env)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInvalidRequest,
			Message: err.Error(),
		}, nil)
		return nil
	}

	files, err := listEditableSkillFiles(baseDir)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInternal,
			Message: "failed to list skill files: " + err.Error(),
		}, nil)
		return nil
	}

	sort.Strings(files)
	opts.Respond(true, map[string]interface{}{
		"files": files,
	}, nil, nil)
	return nil
}

// SkillsGetFileParams holds parameters for skills.getFile.
type SkillsGetFileParams struct {
	SkillKey string
	FilePath string
}

func parseSkillsGetFileParams(params map[string]interface{}) (*SkillsGetFileParams, error) {
	p := &SkillsGetFileParams{}
	skillKey, ok := params["skillKey"].(string)
	if !ok || strings.TrimSpace(skillKey) == "" {
		return nil, fmt.Errorf("skillKey is required")
	}
	p.SkillKey = strings.TrimSpace(skillKey)
	filePath, ok := params["filePath"].(string)
	if !ok || strings.TrimSpace(filePath) == "" {
		return nil, fmt.Errorf("filePath is required")
	}
	p.FilePath = strings.TrimSpace(filePath)
	return p, nil
}

// SkillsGetFileHandler handles "skills.getFile".
func SkillsGetFileHandler(opts HandlerOpts) error {
	params, err := parseSkillsGetFileParams(opts.Params)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInvalidRequest,
			Message: fmt.Sprintf("invalid skills.getFile params: %v", err),
		}, nil)
		return nil
	}

	env := func(k string) string { return os.Getenv(k) }
	cfg, err := config.Load(env)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInternal,
			Message: "failed to load config: " + err.Error(),
		}, nil)
		return nil
	}

	baseDir, err := resolveSkillBaseDir(cfg, params.SkillKey, env)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInvalidRequest,
			Message: err.Error(),
		}, nil)
		return nil
	}

	absPath, err := sanitizeSkillFilePath(baseDir, params.FilePath)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInvalidRequest,
			Message: err.Error(),
		}, nil)
		return nil
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInternal,
			Message: "failed to read file: " + err.Error(),
		}, nil)
		return nil
	}

	opts.Respond(true, map[string]interface{}{
		"content": string(content),
	}, nil, nil)
	return nil
}

// SkillsSaveFileParams holds parameters for skills.saveFile.
type SkillsSaveFileParams struct {
	SkillKey string
	FilePath string
	Content  string
}

func parseSkillsSaveFileParams(params map[string]interface{}) (*SkillsSaveFileParams, error) {
	p := &SkillsSaveFileParams{}
	skillKey, ok := params["skillKey"].(string)
	if !ok || strings.TrimSpace(skillKey) == "" {
		return nil, fmt.Errorf("skillKey is required")
	}
	p.SkillKey = strings.TrimSpace(skillKey)
	filePath, ok := params["filePath"].(string)
	if !ok || strings.TrimSpace(filePath) == "" {
		return nil, fmt.Errorf("filePath is required")
	}
	p.FilePath = strings.TrimSpace(filePath)
	content, ok := params["content"].(string)
	if !ok {
		return nil, fmt.Errorf("content is required")
	}
	p.Content = content
	return p, nil
}

// SkillsSaveFileHandler handles "skills.saveFile".
func SkillsSaveFileHandler(opts HandlerOpts) error {
	params, err := parseSkillsSaveFileParams(opts.Params)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInvalidRequest,
			Message: fmt.Sprintf("invalid skills.saveFile params: %v", err),
		}, nil)
		return nil
	}

	env := func(k string) string { return os.Getenv(k) }
	cfg, err := config.Load(env)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInternal,
			Message: "failed to load config: " + err.Error(),
		}, nil)
		return nil
	}

	baseDir, err := resolveSkillBaseDir(cfg, params.SkillKey, env)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInvalidRequest,
			Message: err.Error(),
		}, nil)
		return nil
	}

	absPath, err := sanitizeSkillFilePath(baseDir, params.FilePath)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInvalidRequest,
			Message: err.Error(),
		}, nil)
		return nil
	}

	contentBytes := []byte(params.Content)

	// Validate syntax before saving
	if err := validateFileSyntax(absPath, contentBytes); err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInvalidRequest,
			Message: err.Error(),
		}, nil)
		return nil
	}

	// Backup original file if it exists
	backupPath := absPath + ".openocta-backup"
	originalExists := false
	if _, err := os.Stat(absPath); err == nil {
		originalExists = true
		originalData, err := os.ReadFile(absPath)
		if err != nil {
			opts.Respond(false, nil, &protocol.ErrorShape{
				Code:    protocol.ErrCodeInternal,
				Message: "failed to read original file for backup: " + err.Error(),
			}, nil)
			return nil
		}
		if err := os.WriteFile(backupPath, originalData, 0600); err != nil {
			opts.Respond(false, nil, &protocol.ErrorShape{
				Code:    protocol.ErrCodeInternal,
				Message: "failed to create backup: " + err.Error(),
			}, nil)
			return nil
		}
	}

	// Write new content
	if err := os.WriteFile(absPath, contentBytes, 0644); err != nil {
		// Attempt to restore from backup
		if originalExists {
			_ = os.Rename(backupPath, absPath)
		}
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInternal,
			Message: "failed to save file: " + err.Error(),
		}, nil)
		return nil
	}

	// Verify the file was written correctly
	written, err := os.ReadFile(absPath)
	if err != nil || string(written) != params.Content {
		if originalExists {
			_ = os.Rename(backupPath, absPath)
		}
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInternal,
			Message: "file verification failed after save",
		}, nil)
		return nil
	}

	// Remove backup on success
	if originalExists {
		_ = os.Remove(backupPath)
	}

	// Reload skills to ensure the change is picked up immediately.
	// LoadWorkspaceEntries reads from disk each time, so triggering a load validates freshness.
	workspaceDir := agent.ResolveAgentWorkspaceDir(cfg, resolveDefaultAgentID(cfg), env)
	_ = loadWorkspaceSkillEntries(workspaceDir, cfg)

	opts.Respond(true, map[string]interface{}{
		"ok":       true,
		"skillKey": params.SkillKey,
		"filePath": params.FilePath,
	}, nil, nil)
	return nil
}
