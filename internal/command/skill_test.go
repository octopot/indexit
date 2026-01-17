package command_test

import (
	"bytes"
	"encoding/json"
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
