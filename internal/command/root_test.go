package command_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "go.octolab.org/toolset/indexit/internal/command"
)

func TestNew(t *testing.T) {
	root := New()
	require.NotNil(t, root)
	assert.NotEmpty(t, root.Use)
	assert.NotEmpty(t, root.Short)
	assert.NotEmpty(t, root.Long)
	assert.False(t, root.SilenceErrors)
	assert.True(t, root.SilenceUsage)
}

func TestAuthStatusLogsProxyOnce(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("INDEXIT_PROXY_URL", "socks5://127.0.0.1:1")
	t.Setenv("TELEGRAM_API_ID", "1")
	t.Setenv("TELEGRAM_API_HASH", "hash")
	session := filepath.Join(dir, "session.json")
	status := func() string {
		var stderr bytes.Buffer
		root := New()
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&stderr)
		root.SetArgs([]string{"--env-file", "", "telegram",
			"--session", session, "--peer-cache", filepath.Join(dir, "peers.json"), "--timeout", "300ms",
			"auth", "status"})
		_ = root.Execute()
		return stderr.String()
	}

	assert.Equal(t, 1, strings.Count(status(), "proxy type="), "without a session no client is built")

	require.NoError(t, os.WriteFile(session, []byte("{}"), 0o600))
	assert.Equal(t, 1, strings.Count(status(), "proxy type="), "the client logs it, the command does not")
}
