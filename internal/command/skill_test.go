package command_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "go.octolab.org/toolset/indexit/internal/command"
	"go.octolab.org/toolset/indexit/internal/exitcode"
	"go.octolab.org/toolset/indexit/internal/skill"
	"go.octolab.org/toolset/indexit/skills"
)

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := New()
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

// sandbox points every agent directory into a temporary home.
func sandbox(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	t.Chdir(t.TempDir())
	return home
}

func TestSkill_offline(t *testing.T) {
	// Neither .env nor its flag is read.
	out, err := run(t, "--env-file", filepath.Join(t.TempDir(), "missing"), "skill", "show")
	require.NoError(t, err)
	assert.Contains(t, out, "name: indexit")

	out, err = run(t, "skill", "show", "--file", "references/cli.md")
	require.NoError(t, err)
	assert.Contains(t, out, "## indexit skill install")

	_, err = run(t, "skill", "show", "--file", "missing.md")
	assert.Equal(t, exitcode.Usage, exitcode.FromError(err))
}

func TestSkill_info(t *testing.T) {
	out, err := run(t, "skill", "info", "--json")
	require.NoError(t, err)
	var info map[string]string
	require.NoError(t, json.Unmarshal([]byte(out), &info))
	digest, err := skill.Digest(skills.Indexit())
	require.NoError(t, err)
	assert.Equal(t, digest, info["digest"])
	assert.Equal(t, skill.DigestAlgorithm, info["digest_algorithm"])
	assert.Equal(t, "indexit", info["name"])
	assert.NotEmpty(t, info["version"])
	assert.Contains(t, info["tool_version_range"], ">="+info["version"])
}

func TestSkill_installAndStatus(t *testing.T) {
	home := sandbox(t)

	_, err := run(t, "skill", "status", "--agent", "codex")
	assert.Equal(t, exitcode.Fail, exitcode.FromError(err), "a requested agent without a copy fails")

	out, err := run(t, "skill", "install", "--agent", "claude-code,codex")
	require.NoError(t, err)
	assert.Contains(t, out, filepath.Join(home, ".claude", "skills", "indexit"))
	assert.Contains(t, out, filepath.Join(home, ".agents", "skills", "indexit"))

	out, err = run(t, "skill", "install", "--agent", "codex")
	require.NoError(t, err)
	assert.Contains(t, out, "already holds")

	out, err = run(t, "skill", "status", "--json", "--exact")
	require.NoError(t, err)
	var copies []skill.Copy
	for _, line := range bytes.Split(bytes.TrimSpace([]byte(out)), []byte("\n")) {
		var c skill.Copy
		require.NoError(t, json.Unmarshal(line, &c))
		copies = append(copies, c)
	}
	require.Len(t, copies, 2)
	for _, c := range copies {
		assert.Equal(t, "exact", c.ReleaseMatch)
		assert.Equal(t, "intact", c.Integrity)
		assert.Equal(t, "indexit", c.Ownership)
	}

	_, err = run(t, "skill", "install", "--agent", "claude-code", "--project")
	require.NoError(t, err)
	out, err = run(t, "skill", "status", "--agent", "claude-code")
	require.NoError(t, err)
	assert.Contains(t, out, "Claude Code loads 2 copies")

	_, err = run(t, "skill", "install", "--agent", "cursor")
	assert.Equal(t, exitcode.Usage, exitcode.FromError(err))
}

func TestSkill_statusCodexPlugin(t *testing.T) {
	home := sandbox(t)
	codex := filepath.Join(home, ".codex")
	dir := filepath.Join(codex, "plugins", "cache", "octolab", "indexit", "0.2.2", "skills", "indexit")
	require.NoError(t, os.MkdirAll(filepath.Dir(dir), 0o755))
	require.NoError(t, os.CopyFS(dir, skills.Indexit()))
	require.NoError(t, os.WriteFile(filepath.Join(codex, "config.toml"),
		[]byte("[plugins.\"indexit@octolab\"]\nenabled = true\n"), 0o644))

	out, err := run(t, "skill", "status", "--agent", "codex", "--exact")
	require.NoError(t, err, "the plugin copy is the agent's copy")
	assert.Contains(t, out, "Codex plugin: indexit@octolab ~/.codex/plugins/cache/octolab/indexit/0.2.2/skills/indexit")
	assert.Contains(t, out, "same as this release, installed by another tool")

	_, err = run(t, "skill", "install", "--agent", "codex")
	require.NoError(t, err)
	out, err = run(t, "skill", "status", "--agent", "codex")
	require.NoError(t, err)
	assert.Contains(t, out, "Codex loads 2 copies")
}

func TestSkill_statusAmbiguousCodexPlugin(t *testing.T) {
	home := sandbox(t)
	codex := filepath.Join(home, ".codex")
	for _, version := range []string{"1.9.0", "1.10.0", "1.11x"} {
		dir := filepath.Join(codex, "plugins", "cache", "octolab", "indexit", version, "skills", "indexit")
		require.NoError(t, os.MkdirAll(filepath.Dir(dir), 0o755))
		require.NoError(t, os.CopyFS(dir, skills.Indexit()))
	}
	require.NoError(t, os.WriteFile(filepath.Join(codex, "config.toml"),
		[]byte("[plugins.\"indexit@octolab\"]\n"), 0o644))

	out, err := run(t, "skill", "status", "--agent", "codex")
	require.NoError(t, err, "without --exact an ambiguous copy is only reported")
	assert.Contains(t, out, "Codex plugin: indexit@octolab")
	assert.Contains(t, out, "match with this release unknown")
	assert.Contains(t, out, "Codex's pick among 1.10.0, 1.11x, 1.9.0 depends on the order on disk; remove the stale ones")

	_, err = run(t, "skill", "status", "--agent", "codex", "--exact")
	assert.Equal(t, exitcode.Fail, exitcode.FromError(err), "an ambiguous copy is never an exact match")

	out, err = run(t, "skill", "status", "--agent", "codex", "--json")
	require.NoError(t, err)
	var c skill.Copy
	require.NoError(t, json.Unmarshal([]byte(out), &c))
	assert.Equal(t, []string{"1.10.0", "1.11x", "1.9.0"}, c.AmbiguousVersions)
	assert.Equal(t, "unknown", c.ReleaseMatch)
}

func TestSkill_statusAmbiguousCodexPluginWithoutCopy(t *testing.T) {
	home := sandbox(t)
	codex := filepath.Join(home, ".codex")
	cache := filepath.Join(codex, "plugins", "cache", "octolab", "indexit")
	for _, version := range []string{"1.10.0", "1.11x"} {
		dir := filepath.Join(cache, version, "skills", "indexit")
		require.NoError(t, os.MkdirAll(filepath.Dir(dir), 0o755))
		require.NoError(t, os.CopyFS(dir, skills.Indexit()))
	}
	// The pick of the cycle holds no skill at all.
	require.NoError(t, os.MkdirAll(filepath.Join(cache, "1.9.0"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(codex, "config.toml"),
		[]byte("[plugins.\"indexit@octolab\"]\n"), 0o644))
	_, err := run(t, "skill", "install", "--agent", "codex")
	require.NoError(t, err)

	out, err := run(t, "skill", "status", "--agent", "codex", "--json")
	require.NoError(t, err)
	var plugin *skill.Copy
	for _, line := range bytes.Split(bytes.TrimSpace([]byte(out)), []byte("\n")) {
		var c skill.Copy
		require.NoError(t, json.Unmarshal(line, &c))
		if c.Scope == "plugin" {
			plugin = &c
		}
	}
	require.NotNil(t, plugin, "the ambiguous plugin stays listed without a copy")
	assert.Equal(t, filepath.Join(cache, "1.9.0", "skills", "indexit"), plugin.Path)
	assert.Equal(t, []string{"1.10.0", "1.11x", "1.9.0"}, plugin.AmbiguousVersions)

	out, err = run(t, "skill", "status", "--agent", "codex", "--exact")
	assert.Equal(t, exitcode.Fail, exitcode.FromError(err), "an exact user copy does not hide the ambiguity")
	assert.Contains(t, out, "unreadable SKILL.md")
	assert.Contains(t, out, "depends on the order on disk")
}
