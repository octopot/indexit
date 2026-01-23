package telegram

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.octolab.org/toolset/indexit/internal/exitcode"
)

func TestDebugUID(t *testing.T) {
	var out bytes.Buffer
	command := New()
	command.SetOut(&out)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"debug", "uid", "@telegram"})

	require.NoError(t, command.Execute())
	assert.Contains(t, out.String(), `"Username": "telegram"`)
}

func TestAuthStatusWithoutSession(t *testing.T) {
	var out bytes.Buffer
	command := New()
	command.SetOut(&out)
	command.SetErr(&bytes.Buffer{})
	dir := t.TempDir()
	command.SetArgs([]string{
		"--session", filepath.Join(dir, "session.json"),
		"--peer-cache", filepath.Join(dir, "peers.json"),
		"auth", "status",
	})

	require.NoError(t, command.Execute())
	assert.Contains(t, out.String(), "authorized: false")
	assert.Contains(t, out.String(), "peer cache: "+filepath.Join(dir, "peers.json"))
	assert.Contains(t, out.String(), "peer cache account: none")
}

func TestAuthStatusReportsUnreadablePeerCache(t *testing.T) {
	var out bytes.Buffer
	command := New()
	command.SetOut(&out)
	command.SetErr(&bytes.Buffer{})
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "peers.json"), []byte("{not json"), 0600))
	command.SetArgs([]string{
		"--session", filepath.Join(dir, "session.json"),
		"--peer-cache", filepath.Join(dir, "peers.json"),
		"auth", "status",
	})

	require.NoError(t, command.Execute(), "a corrupt cache does not fail status")
	assert.Contains(t, out.String(), "peer cache account: unreadable (decode peer cache:")
}

func TestFetchMessagesBadUIDIsUsageError(t *testing.T) {
	command := New()
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"fetch", "messages", "--dialog", "123"})

	err := command.Execute()
	require.Error(t, err)

	var usage *exitcode.Error
	require.True(t, errors.As(err, &usage))
	assert.Equal(t, exitcode.Usage, usage.Code)
}
