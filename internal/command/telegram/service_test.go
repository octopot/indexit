package telegram

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.octolab.org/toolset/indexit/internal/config"
	"go.octolab.org/toolset/indexit/internal/exitcode"
	"go.octolab.org/toolset/indexit/internal/preview"
)

func TestServiceRunDefaults(t *testing.T) {
	assert.Equal(t, []string{"127.0.0.1:17840", "[::1]:17840"}, Listen)
	run, _, err := NewService().Find([]string{"run"})
	require.NoError(t, err)
	flag := run.Flags().Lookup("listen")
	require.NotNil(t, flag)
	assert.Equal(t, "stringArray", flag.Value.Type())
	assert.Equal(t, "[127.0.0.1:17840,[::1]:17840]", flag.DefValue)
	assert.Equal(t, "http://indexit.localhost:"+strconv.Itoa(Port), preview.DefaultOrigin, "URLs reach the default port")
}

func TestServiceRunRejectsNonLoopbackAddresses(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("INDEXIT_DATA_DIR", t.TempDir())
	// An address that slips through would serve until the deadline instead.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, listen := range []string{"0.0.0.0:17840", "localhost:17840", "192.0.2.1:17840", ":17840", "127.0.0.1", "[::]:17840", "[::1%lo0]:17840", "", "127.0.0.1:", "127.0.0.1:http", "127.0.0.1:65536", "127.0.0.1:-1", "[::1]:+80"} {
		cmd := NewService()
		cmd.SetArgs([]string{"run", "--listen", "127.0.0.1:0", "--listen", listen})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		err := cmd.ExecuteContext(ctx)
		require.Error(t, err, listen)
		var usage *exitcode.Error
		require.True(t, errors.As(err, &usage), listen)
		assert.Equal(t, exitcode.Usage, usage.Code, listen)
	}
}

// startService runs the service with args and returns the addresses it
// reports and the result of the run.
func startService(t *testing.T, ctx context.Context, args ...string) ([]string, <-chan error) {
	t.Helper()
	return startServiceWith(t, ctx, t.TempDir(), args...)
}

// startServiceWith is startService with the given XDG_CONFIG_HOME.
func startServiceWith(t *testing.T, ctx context.Context, configHome string, args ...string) ([]string, <-chan error) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("INDEXIT_DATA_DIR", t.TempDir())
	stderr, logs := io.Pipe()
	cmd := NewService()
	cmd.SetArgs(append([]string{"run"}, args...))
	cmd.SetOut(io.Discard)
	cmd.SetErr(logs)
	done := make(chan error, 1)
	go func() { done <- cmd.ExecuteContext(ctx); logs.Close() }()
	line, err := bufio.NewReader(stderr).ReadString('\n')
	require.NoError(t, err)
	go func() { _, _ = io.Copy(io.Discard, stderr) }()
	match := regexp.MustCompile(`^image service: (.+) \(URLs: http://indexit\.localhost:17840, data: .+\)\n$`).FindStringSubmatch(line)
	require.Len(t, match, 2, line)
	return strings.Split(match[1], ", "), done
}

func TestServiceRunServesEveryAddressUntilCancelled(t *testing.T) {
	args := []string{"--listen", "127.0.0.1:0"}
	// IPv6 joins in where the system has it.
	if probe, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "[::1]:0"); err == nil {
		require.NoError(t, probe.Close())
		args = append(args, "--listen", "[::1]:0")
	} else {
		require.True(t, unavailable(err), err)
		t.Log("IPv6 loopback is unavailable:", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addresses, done := startService(t, ctx, args...)
	require.Len(t, addresses, len(args)/2)
	assert.True(t, strings.HasPrefix(addresses[0], "127.0.0.1:"), addresses[0])
	if len(addresses) > 1 {
		assert.True(t, strings.HasPrefix(addresses[1], "[::1]:"), addresses[1])
	}
	for _, address := range addresses {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+address+"/", http.NoBody)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err, address)
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode, address)
		assert.Equal(t, "indexit image service is running\n", string(body), address)
	}
	cancel()
	assert.NoError(t, <-done)
}

func TestServiceRunWritesTheDefaultConfig(t *testing.T) {
	configHome := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	_, done := startServiceWith(t, ctx, configHome, "--listen", "127.0.0.1:0")
	cancel()
	require.NoError(t, <-done)
	data, err := os.ReadFile(filepath.Join(configHome, "indexit", "config.toml"))
	require.NoError(t, err)
	assert.Equal(t, config.Template, data)
}

func TestServiceRunStartsWithoutAWritableConfigDirectory(t *testing.T) {
	configHome := t.TempDir()
	require.NoError(t, os.Chmod(configHome, 0o500))
	t.Cleanup(func() { _ = os.Chmod(configHome, 0o700) })
	ctx, cancel := context.WithCancel(context.Background())
	_, done := startServiceWith(t, ctx, configHome, "--listen", "127.0.0.1:0")
	cancel()
	assert.NoError(t, <-done)
}

func TestServiceRunRejectsMalformedConfig(t *testing.T) {
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("INDEXIT_DATA_DIR", t.TempDir())
	path := filepath.Join(config, "indexit", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("[service]\norigin = \"http://indexit.localhost:17840/images\"\n"), 0o600))
	cmd := NewService()
	cmd.SetArgs([]string{"run", "--listen", "127.0.0.1:0"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.ExecuteContext(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), path+": service.origin")
	var usage *exitcode.Error
	require.True(t, errors.As(err, &usage))
	assert.Equal(t, exitcode.Usage, usage.Code)
}

func TestServiceRunReportsBusyAddress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	addresses, done := startService(t, ctx, "--listen", "127.0.0.1:0")
	defer func() { cancel(); assert.NoError(t, <-done) }()

	second := NewService()
	second.SetArgs([]string{"run", "--listen", addresses[0]})
	second.SetOut(io.Discard)
	second.SetErr(io.Discard)
	err := second.ExecuteContext(context.Background())
	require.Error(t, err)
	var usage *exitcode.Error
	assert.False(t, errors.As(err, &usage), "a busy address is not a usage error")
}

func TestListenAll(t *testing.T) {
	ctx := context.Background()
	// ::2 is not assigned to any interface, as ::1 is not with IPv6 turned off.
	missing := "[::2]:0"

	t.Run("optional address the system lacks", func(t *testing.T) {
		var skipped []string
		listeners, err := listenAll(ctx, []string{"127.0.0.1:0", missing}, true, func(address string, err error) {
			assert.True(t, unavailable(err), err)
			skipped = append(skipped, address)
		})
		require.NoError(t, err)
		require.Len(t, listeners, 1)
		assert.Equal(t, []string{missing}, skipped)
		require.NoError(t, listeners[0].Close())
	})

	t.Run("explicit address the system lacks", func(t *testing.T) {
		_, err := listenAll(ctx, []string{"127.0.0.1:0", missing}, false, func(string, error) { t.Error("no skip") })
		assert.Error(t, err)
	})

	t.Run("busy port is not skipped", func(t *testing.T) {
		busy := loopback(t)
		defer busy.Close()
		_, err := listenAll(ctx, []string{busy.Addr().String()}, true, func(string, error) { t.Error("no skip") })
		assert.Error(t, err)
	})

	t.Run("nothing bound", func(t *testing.T) {
		_, err := listenAll(ctx, []string{missing}, true, func(string, error) {})
		assert.Error(t, err)
	})

	t.Run("a later failure closes the earlier listeners", func(t *testing.T) {
		busy := loopback(t)
		defer busy.Close()
		free := loopback(t)
		address := free.Addr().String()
		require.NoError(t, free.Close())
		_, err := listenAll(ctx, []string{address, busy.Addr().String()}, false, func(string, error) {})
		require.Error(t, err)
		again, err := listenAll(ctx, []string{address}, false, func(string, error) {})
		require.NoError(t, err, "the first address is free again")
		require.NoError(t, again[0].Close())
	})
}

// brokenListener fails to accept, as a listener whose socket went away.
type brokenListener struct{ net.Listener }

func (brokenListener) Accept() (net.Conn, error) { return nil, errors.New("accept: broken") }

func TestServeStopsEveryListenerOnFailure(t *testing.T) {
	healthy := loopback(t)
	broken := loopback(t)
	defer broken.Close()
	err := serve(context.Background(), []net.Listener{healthy, brokenListener{broken}}, http.NotFoundHandler(), time.Second)
	require.Error(t, err)
	assert.NotErrorIs(t, err, http.ErrServerClosed)
	_, dial := (&net.Dialer{Timeout: time.Second}).DialContext(context.Background(), "tcp", healthy.Addr().String())
	assert.Error(t, dial, "the healthy listener is closed too")
}

func loopback(t *testing.T) net.Listener {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	return listener
}
