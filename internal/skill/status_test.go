package skill_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "go.octolab.org/toolset/indexit/internal/skill"
)

func writeSkill(t *testing.T, dir, text string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(text), 0o644))
}

func TestLocations(t *testing.T) {
	root := t.TempDir()
	env := Env{
		Home:      filepath.Join(root, "home"),
		ClaudeDir: filepath.Join(root, "claude"),
		CodexHome: filepath.Join(root, "codex"),
		Cwd:       filepath.Join(root, "repo", "sub"),
	}
	require.NoError(t, os.MkdirAll(filepath.Join(root, "repo", ".git"), 0o755))
	for _, dir := range []string{
		filepath.Join(env.ClaudeDir, "skills", "indexit"),
		filepath.Join(root, "repo", ".claude", "skills", "indexit"),
		filepath.Join(env.Cwd, ".agents", "skills", "indexit"),
		filepath.Join(env.Home, ".agents", "skills", "indexit"),
		filepath.Join(env.CodexHome, "skills", "indexit"),
		filepath.Join(root, "cache", "user", "skills", "indexit"),
		filepath.Join(root, "cache", "here", "skills", "indexit"),
		filepath.Join(root, "cache", "elsewhere", "skills", "indexit"),
		filepath.Join(root, "cache", "other", "skills", "indexit"),
	} {
		writeSkill(t, dir, "x")
	}
	registry := map[string]any{"version": 2, "plugins": map[string]any{
		"indexit@octolab": []map[string]string{
			{"scope": "user", "installPath": filepath.Join(root, "cache", "user")},
			{"scope": "project", "projectPath": filepath.Join(root, "repo"), "installPath": filepath.Join(root, "cache", "here")},
			{"scope": "project", "projectPath": filepath.Join(root, "elsewhere"), "installPath": filepath.Join(root, "cache", "elsewhere")},
		},
		"other@octolab": []map[string]string{{"scope": "user", "installPath": filepath.Join(root, "cache", "other")}},
	}}
	data, err := json.Marshal(registry)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(env.ClaudeDir, "plugins"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(env.ClaudeDir, "plugins", "installed_plugins.json"), data, 0o644))

	var got [][3]string
	for _, loc := range Locations(env, "indexit") {
		rel, err := filepath.Rel(root, loc.Path)
		require.NoError(t, err)
		got = append(got, [3]string{loc.Agent, loc.Scope, rel})
	}
	assert.Equal(t, [][3]string{
		{ClaudeCode, "user", "claude/skills/indexit"},
		{ClaudeCode, "project", "repo/.claude/skills/indexit"},
		{ClaudeCode, "plugin", "cache/here/skills/indexit"},
		{ClaudeCode, "plugin", "cache/user/skills/indexit"},
		{Codex, "user", "home/.agents/skills/indexit"},
		{Codex, "project", "repo/sub/.agents/skills/indexit"},
		{Codex, "legacy", "codex/skills/indexit"},
	}, got)
}

func TestLocations_home(t *testing.T) {
	home := t.TempDir()
	env := Env{Home: home, ClaudeDir: filepath.Join(home, ".claude"), CodexHome: filepath.Join(home, ".codex"), Cwd: home}
	writeSkill(t, filepath.Join(home, ".claude", "skills", "indexit"), "x")
	locations := Locations(env, "indexit")
	require.Len(t, locations, 1)
	assert.Equal(t, "user", locations[0].Scope)
}

func TestInspect(t *testing.T) {
	v1, m1 := release(t, "0.2.0", skillMD)
	dir := filepath.Join(t.TempDir(), "indexit")
	_, err := Export(v1, dir, m1, false)
	require.NoError(t, err)
	loc := Location{Agent: ClaudeCode, Scope: "user", Path: dir}

	c := Inspect(loc, m1.Digest, "0.2.3")
	assert.Equal(t, "0.2.0", c.Version)
	assert.Equal(t, ">=0.2.0 <0.3.0", c.Range)
	assert.Equal(t, "exact", c.ReleaseMatch)
	assert.Equal(t, "compatible", c.Compatibility)
	assert.Equal(t, "intact", c.Integrity)
	assert.Equal(t, "indexit", c.Ownership)

	assert.Equal(t, "incompatible", Inspect(loc, m1.Digest, "0.3.0").Compatibility)
	assert.Equal(t, "unknown", Inspect(loc, m1.Digest, "dev").Compatibility)
	assert.Equal(t, "different", Inspect(loc, "sha256:other", "0.2.0").ReleaseMatch)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "references", "a.md"), []byte("b"), 0o644))
	assert.Equal(t, "modified", Inspect(loc, m1.Digest, "0.2.0").Integrity)

	link := filepath.Join(t.TempDir(), "indexit")
	require.NoError(t, os.Symlink(dir, link))
	c = Inspect(Location{Agent: Codex, Scope: "user", Path: link}, m1.Digest, "0.2.0")
	assert.True(t, c.Symlink)
	assert.Equal(t, "external", c.Ownership)
	assert.Equal(t, "unknown", c.Integrity)

	foreign := filepath.Join(t.TempDir(), "indexit")
	writeSkill(t, foreign, "no frontmatter")
	c = Inspect(Location{Agent: Codex, Scope: "user", Path: foreign}, m1.Digest, "0.2.0")
	assert.Equal(t, "external", c.Ownership)
	assert.Equal(t, "unknown", c.Compatibility)
	assert.Empty(t, c.Version)
}

func TestEnv_Dir(t *testing.T) {
	env := Env{Home: "/h", ClaudeDir: "/c", CodexHome: "/x", Cwd: "/p"}
	assert.Equal(t, "/c/skills/indexit", env.Dir(ClaudeCode, "indexit", false))
	assert.Equal(t, "/p/.claude/skills/indexit", env.Dir(ClaudeCode, "indexit", true))
	assert.Equal(t, "/h/.agents/skills/indexit", env.Dir(Codex, "indexit", false))
	assert.Equal(t, "/p/.agents/skills/indexit", env.Dir(Codex, "indexit", true))
}
