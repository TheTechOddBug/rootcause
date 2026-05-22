package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"rootcause/internal/config"
	"rootcause/internal/skills/catalog"
)

const maxSkillGuidanceBytes = 4000

type customSkillCandidate struct {
	Guidance SkillGuidance
	Tags     []string
}

type fileState struct {
	Path    string
	Exists  bool
	ModTime int64
	Size    int64
}

type customSkillCacheEntry struct {
	DirStates  []fileState
	FileStates []fileState
	Candidates []customSkillCandidate
}

type customSkillCache struct {
	mu      sync.Mutex
	entries map[string]customSkillCacheEntry
}

func newCustomSkillCache() *customSkillCache {
	return &customSkillCache{entries: map[string]customSkillCacheEntry{}}
}

func customSkillGuidanceForTool(cfg *config.Config, spec ToolSpec, args map[string]any, cache *customSkillCache) ([]SkillGuidance, error) {
	if cfg == nil || len(cfg.Skills.CustomDirs) == 0 {
		return nil, nil
	}
	candidates, err := cachedCustomSkillCandidates(cfg.Skills.CustomDirs, cfg.Skills.AllowCustomOverrides, cache)
	if err != nil {
		return nil, err
	}
	callTags := toolCallTags(spec, args)
	guidance := make([]SkillGuidance, 0)
	for _, candidate := range candidates {
		if !tagsIntersect(callTags, candidate.Tags) {
			continue
		}
		guidance = append(guidance, cloneSkillGuidance(candidate.Guidance))
	}
	return guidance, nil
}

func cachedCustomSkillCandidates(dirs []string, allowOverrides bool, cache *customSkillCache) ([]customSkillCandidate, error) {
	if cache == nil {
		cache = newCustomSkillCache()
	}
	key := customSkillCacheKey(dirs, allowOverrides)
	dirStates, err := statSkillDirs(dirs)
	if err != nil {
		return nil, err
	}
	cache.mu.Lock()
	entry, ok := cache.entries[key]
	if ok && fileStatesEqual(entry.DirStates, dirStates) && fileStatesStillCurrent(entry.FileStates) {
		out := cloneCustomSkillCandidates(entry.Candidates)
		cache.mu.Unlock()
		return out, nil
	}
	cache.mu.Unlock()

	manifest, err := catalog.LoadWithCustom(catalog.CustomOptions{Dirs: dirs, AllowOverrides: allowOverrides})
	if err != nil {
		return nil, err
	}
	candidates := make([]customSkillCandidate, 0)
	fileStates := make([]fileState, 0)
	for _, skill := range manifest.Skills {
		if !skill.Custom {
			continue
		}
		data, err := os.ReadFile(skill.Path)
		if err != nil {
			return nil, fmt.Errorf("read custom skill %s: %w", skill.Name, err)
		}
		state, err := statPath(skill.Path)
		if err != nil {
			return nil, err
		}
		fileStates = append(fileStates, state)
		content := string(data)
		truncated := false
		if len(content) > maxSkillGuidanceBytes {
			content = content[:maxSkillGuidanceBytes]
			truncated = true
		}
		guidance := SkillGuidance{Name: skill.Name, Description: skill.Description, Tags: append([]string{}, skill.Tags...), Content: content, Truncated: truncated}
		candidates = append(candidates, customSkillCandidate{Guidance: guidance, Tags: append([]string{}, skill.Tags...)})
	}

	cache.mu.Lock()
	cache.entries[key] = customSkillCacheEntry{DirStates: dirStates, FileStates: fileStates, Candidates: cloneCustomSkillCandidates(candidates)}
	cache.mu.Unlock()
	return candidates, nil
}

func customSkillCacheKey(dirs []string, allowOverrides bool) string {
	return fmt.Sprintf("%t\x00%s", allowOverrides, strings.Join(dirs, "\x00"))
}

func statSkillDirs(dirs []string) ([]fileState, error) {
	states := make([]fileState, 0, len(dirs))
	for _, dir := range dirs {
		resolved, err := resolveSkillPath(dir)
		if err != nil {
			return nil, err
		}
		state, err := statPath(resolved)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}

func resolveSkillPath(path string) (string, error) {
	trimmed := strings.TrimSpace(os.ExpandEnv(path))
	if trimmed == "" {
		return "", nil
	}
	if strings.HasPrefix(trimmed, "~/") || trimmed == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		if trimmed == "~" {
			trimmed = home
		} else {
			trimmed = filepath.Join(home, strings.TrimPrefix(trimmed, "~/"))
		}
	}
	return filepath.Abs(trimmed)
}

func statPath(path string) (fileState, error) {
	state := fileState{Path: path}
	if strings.TrimSpace(path) == "" {
		return state, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, err
	}
	state.Exists = true
	state.ModTime = info.ModTime().UnixNano()
	state.Size = info.Size()
	return state, nil
}

func fileStatesStillCurrent(states []fileState) bool {
	for _, previous := range states {
		current, err := statPath(previous.Path)
		if err != nil || !fileStateEqual(previous, current) {
			return false
		}
	}
	return true
}

func fileStatesEqual(left []fileState, right []fileState) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if !fileStateEqual(left[idx], right[idx]) {
			return false
		}
	}
	return true
}

func fileStateEqual(left fileState, right fileState) bool {
	return left.Path == right.Path && left.Exists == right.Exists && left.ModTime == right.ModTime && left.Size == right.Size
}

func cloneCustomSkillCandidates(in []customSkillCandidate) []customSkillCandidate {
	out := make([]customSkillCandidate, 0, len(in))
	for _, candidate := range in {
		out = append(out, customSkillCandidate{Guidance: cloneSkillGuidance(candidate.Guidance), Tags: append([]string{}, candidate.Tags...)})
	}
	return out
}

func cloneSkillGuidance(in SkillGuidance) SkillGuidance {
	in.Tags = append([]string{}, in.Tags...)
	return in
}

func attachCustomSkillGuidance(result ToolResult, guidance []SkillGuidance, guidanceErr error) ToolResult {
	if len(guidance) > 0 {
		result.Metadata.CustomSkills = guidance
	}
	if guidanceErr != nil {
		result.Metadata.CustomSkillError = guidanceErr.Error()
	}
	root, ok := result.Data.(map[string]any)
	if !ok {
		return result
	}
	if len(guidance) > 0 {
		if _, exists := root["customSkillGuidance"]; !exists {
			root["customSkillGuidance"] = guidance
		}
	}
	if guidanceErr != nil {
		if _, exists := root["customSkillError"]; !exists {
			root["customSkillError"] = guidanceErr.Error()
		}
	}
	return result
}

func toolCallTags(spec ToolSpec, args map[string]any) map[string]struct{} {
	tags := map[string]struct{}{}
	addSkillTag(tags, spec.ToolsetID)
	addSkillTag(tags, "toolset:"+spec.ToolsetID)
	addSkillTag(tags, spec.Name)
	addSkillTag(tags, "tool:"+spec.Name)
	addSkillTag(tags, string(spec.Safety))
	for _, token := range strings.FieldsFunc(spec.Name, func(r rune) bool {
		return r == '.' || r == '_' || r == '-'
	}) {
		addSkillTag(tags, token)
	}
	addTagsFromArg(tags, args["skillTags"])
	addTagsFromArg(tags, args["customSkillTags"])
	return tags
}

func tagsIntersect(callTags map[string]struct{}, skillTags []string) bool {
	for _, tag := range skillTags {
		if _, ok := callTags[normalizeSkillTag(tag)]; ok {
			return true
		}
	}
	return false
}

func addTagsFromArg(tags map[string]struct{}, value any) {
	switch typed := value.(type) {
	case string:
		for _, part := range strings.Split(typed, ",") {
			addSkillTag(tags, part)
		}
	case []string:
		for _, part := range typed {
			addSkillTag(tags, part)
		}
	case []any:
		for _, item := range typed {
			if part, ok := item.(string); ok {
				addSkillTag(tags, part)
			}
		}
	}
}

func addSkillTag(tags map[string]struct{}, tag string) {
	normalized := normalizeSkillTag(tag)
	if normalized != "" {
		tags[normalized] = struct{}{}
	}
}

func normalizeSkillTag(tag string) string {
	return strings.ToLower(strings.TrimSpace(tag))
}
