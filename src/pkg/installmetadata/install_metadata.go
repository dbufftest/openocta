// Package installmetadata provides .install-metadata.json for remote market plugins.
package installmetadata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/openocta/openocta/pkg/paths"
)

const installMetadataFilename = ".install-metadata.json"

// Entry 单条安装记录
type Entry struct {
	RemoteID    string `json:"remoteId"`
	LocalID     string `json:"localId"`
	Kind        string `json:"kind"`
	Type        string `json:"type"`
	InstalledAt string `json:"installedAt"`
}

// Metadata 安装元数据文件结构
type Metadata struct {
	Employees []Entry `json:"employees"`
	Skills    []Entry `json:"skills"`
	Mcps      []Entry `json:"mcps"`
}

var mu sync.RWMutex

func resolvePath(env func(string) string) string {
	stateDir := paths.ResolveStateDir(env)
	return filepath.Join(stateDir, installMetadataFilename)
}

func loadUnlocked(env func(string) string) (*Metadata, error) {
	if env == nil {
		env = func(string) string { return "" }
	}
	data, err := os.ReadFile(resolvePath(env))
	if err != nil {
		if os.IsNotExist(err) {
			return &Metadata{}, nil
		}
		return nil, err
	}
	var m Metadata
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func saveUnlocked(env func(string) string, m *Metadata) error {
	if env == nil {
		env = func(string) string { return "" }
	}
	path := resolvePath(env)
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Load 加载 .install-metadata.json
func Load(env func(string) string) (*Metadata, error) {
	mu.RLock()
	defer mu.RUnlock()
	return loadUnlocked(env)
}

// Append 追加一条安装记录
func Append(env func(string) string, kind, remoteID, localID, typeVal string) error {
	mu.Lock()
	defer mu.Unlock()
	m, err := loadUnlocked(env)
	if err != nil {
		return err
	}
	entry := Entry{
		RemoteID:    strings.TrimSpace(remoteID),
		LocalID:     strings.TrimSpace(localID),
		Kind:        strings.TrimSpace(strings.ToLower(kind)),
		Type:        strings.TrimSpace(typeVal),
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
	}
	kn := entry.Kind
	switch kn {
	case "employee":
		filtered := make([]Entry, 0, len(m.Employees)+1)
		for _, e := range m.Employees {
			if e.RemoteID != entry.RemoteID {
				filtered = append(filtered, e)
			}
		}
		m.Employees = append(filtered, entry)
	case "skill":
		filtered := make([]Entry, 0, len(m.Skills)+1)
		for _, e := range m.Skills {
			if e.RemoteID != entry.RemoteID {
				filtered = append(filtered, e)
			}
		}
		m.Skills = append(filtered, entry)
	case "mcp":
		filtered := make([]Entry, 0, len(m.Mcps)+1)
		for _, e := range m.Mcps {
			if e.RemoteID != entry.RemoteID {
				filtered = append(filtered, e)
			}
		}
		m.Mcps = append(filtered, entry)
	}
	return saveUnlocked(env, m)
}

// RemoveByRemoteID 按 remoteId 移除
func RemoveByRemoteID(env func(string) string, kind, remoteID string) error {
	mu.Lock()
	defer mu.Unlock()
	m, err := loadUnlocked(env)
	if err != nil {
		return err
	}
	remoteID = strings.TrimSpace(remoteID)
	kn := strings.TrimSpace(strings.ToLower(kind))
	switch kn {
	case "employee":
		filtered := make([]Entry, 0, len(m.Employees))
		for _, e := range m.Employees {
			if e.RemoteID != remoteID {
				filtered = append(filtered, e)
			}
		}
		m.Employees = filtered
	case "skill":
		filtered := make([]Entry, 0, len(m.Skills))
		for _, e := range m.Skills {
			if e.RemoteID != remoteID {
				filtered = append(filtered, e)
			}
		}
		m.Skills = filtered
	case "mcp":
		filtered := make([]Entry, 0, len(m.Mcps))
		for _, e := range m.Mcps {
			if e.RemoteID != remoteID {
				filtered = append(filtered, e)
			}
		}
		m.Mcps = filtered
	}
	return saveUnlocked(env, m)
}

// RemoveByLocalID 按 localId 移除（删除员工/MCP 时用）
func RemoveByLocalID(env func(string) string, kind, localID string) error {
	mu.Lock()
	defer mu.Unlock()
	m, err := loadUnlocked(env)
	if err != nil {
		return err
	}
	localID = strings.TrimSpace(localID)
	kn := strings.TrimSpace(strings.ToLower(kind))
	switch kn {
	case "employee":
		filtered := make([]Entry, 0, len(m.Employees))
		for _, e := range m.Employees {
			if e.LocalID != localID {
				filtered = append(filtered, e)
			}
		}
		m.Employees = filtered
	case "skill":
		filtered := make([]Entry, 0, len(m.Skills))
		for _, e := range m.Skills {
			if e.LocalID != localID {
				filtered = append(filtered, e)
			}
		}
		m.Skills = filtered
	case "mcp":
		filtered := make([]Entry, 0, len(m.Mcps))
		for _, e := range m.Mcps {
			if e.LocalID != localID {
				filtered = append(filtered, e)
			}
		}
		m.Mcps = filtered
	}
	return saveUnlocked(env, m)
}

// EmployeeInstallMap 返回 remoteId -> localId
func EmployeeInstallMap(env func(string) string) map[string]string {
	m, err := Load(env)
	if err != nil || m == nil {
		return nil
	}
	out := make(map[string]string, len(m.Employees))
	for _, e := range m.Employees {
		if e.RemoteID != "" && e.LocalID != "" {
			out[e.RemoteID] = e.LocalID
		}
	}
	return out
}

// McpInstallMap 返回 remoteId -> serverKey
func McpInstallMap(env func(string) string) map[string]string {
	m, err := Load(env)
	if err != nil || m == nil {
		return nil
	}
	out := make(map[string]string, len(m.Mcps))
	for _, e := range m.Mcps {
		if e.RemoteID != "" && e.LocalID != "" {
			out[e.RemoteID] = e.LocalID
		}
	}
	return out
}

// SkillInstallSet 返回已安装的远程 skill folder 集合
func SkillInstallSet(env func(string) string) map[string]struct{} {
	m, err := Load(env)
	if err != nil || m == nil {
		return nil
	}
	out := make(map[string]struct{}, len(m.Skills))
	for _, e := range m.Skills {
		if e.RemoteID != "" {
			out[e.RemoteID] = struct{}{}
		}
	}
	return out
}
