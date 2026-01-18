package telegram

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.octolab.org/toolset/indexit/internal/exitcode"
	tgsvc "go.octolab.org/toolset/indexit/internal/telegram"
)

func TestCollectMessageRefs_GroupsLinksByPeer(t *testing.T) {
	groups, err := collectMessageRefs([]string{
		"https://t.me/example_channel/11",
		"https://t.me/example_channel/46",
		"https://t.me/other_channel/1",
		"https://t.me/example_channel/47",
	}, "", nil)
	require.NoError(t, err)
	require.Len(t, groups, 2)
	assert.Equal(t, "@example_channel", groups[0].ref.String())
	assert.Equal(t, []int{11, 46, 47}, groups[0].ids)
	assert.False(t, groups[0].ref.HasAnchor, "group ref must not keep a single link's anchor")
	assert.Equal(t, "@other_channel", groups[1].ref.String())
	assert.Equal(t, []int{1}, groups[1].ids)
}

func TestCollectMessageRefs_DialogWithIDs(t *testing.T) {
	groups, err := collectMessageRefs(nil, "@example_channel", []int{11, 46})
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, []int{11, 46}, groups[0].ids)
}

func TestCollectMessageRefs_DialogAnchorJoinsIDs(t *testing.T) {
	groups, err := collectMessageRefs(nil, "https://t.me/example_channel/46", []int{47})
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, []int{46, 47}, groups[0].ids)
}

func TestCollectMessageRefs_LinkAndDialogShareGroup(t *testing.T) {
	groups, err := collectMessageRefs([]string{"https://t.me/example_channel/11"}, "@example_channel", []int{46})
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, []int{11, 46}, groups[0].ids)
}

func TestCollectMessageRefs_Errors(t *testing.T) {
	cases := map[string]struct {
		args   []string
		dialog string
		ids    []int
	}{
		"empty input":        {},
		"link without id":    {args: []string{"https://t.me/example_channel"}},
		"ids without dialog": {ids: []int{1}},
		"dialog without ids": {dialog: "@example_channel"},
		"non-positive id":    {dialog: "@example_channel", ids: []int{0}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := collectMessageRefs(tc.args, tc.dialog, tc.ids)
			assert.Error(t, err)
		})
	}
}

func TestCollectMessageRefs_TopicsStaySeparate(t *testing.T) {
	groups, err := collectMessageRefs([]string{
		"https://t.me/c/77/7/11",
		"https://t.me/c/77/8/12",
		"https://t.me/c/77/7/13",
	}, "channel:77:7", []int{14})
	require.NoError(t, err)
	require.Len(t, groups, 2)
	assert.Equal(t, "channel:77:7", groups[0].ref.String())
	assert.Equal(t, []int{11, 13, 14}, groups[0].ids)
	assert.Equal(t, "channel:77:8", groups[1].ref.String())
	assert.Equal(t, []int{12}, groups[1].ids)
}

func TestCollectMessageRefs_UsernameCaseSharesGroup(t *testing.T) {
	groups, err := collectMessageRefs([]string{"https://t.me/Example_Channel/11"}, "@example_channel", []int{12})
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, []int{11, 12}, groups[0].ids)
}

func TestCollectMessageRefs_FourLetterUsername(t *testing.T) {
	groups, err := collectMessageRefs([]string{"https://t.me/beta/11", "https://t.me/s/beta/12"}, "@beta", []int{13})
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "@beta", groups[0].ref.String())
	assert.Equal(t, []int{11, 12, 13}, groups[0].ids)
}

func TestFetchMessageInvalidInputIsUsageError(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"https://t.me/example_channel/0"},
		{"https://t.me/example_channel/2147483648"},
		{"--dialog", "https://t.me/example_channel/0", "--id", "1"},
		{"--dialog", "@example_channel", "--id", "-1"},
		{"--dialog", "@example_channel", "--id", "2147483648"},
		{"--dialog", "@example_channel", "--id", "1", "--limit", "-1"},
		{"--dialog", "@example_channel", "--id", "1", "--format", "csv"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			command := New()
			command.SetOut(&bytes.Buffer{})
			command.SetErr(&bytes.Buffer{})
			command.SetArgs(append([]string{"fetch", "message"}, args...))
			err := command.Execute()
			var usage *exitcode.Error
			require.ErrorAs(t, err, &usage)
			assert.Equal(t, exitcode.Usage, usage.Code)
		})
	}
}

func TestFetchInviteDialogIsUsageError(t *testing.T) {
	// Fails before any connection: without credentials a client would fail
	// with a plain error instead.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("TELEGRAM_API_ID", "")
	for _, args := range [][]string{
		{"messages", "--dialog", "https://t.me/+AbCdEf"},
		{"topics", "--dialog", "https://t.me/+AbCdEf"},
		{"media", "--dialog", "https://t.me/joinchat/AbCdEf", "--dir", t.TempDir()},
		{"message", "--dialog", "https://t.me/+AbCdEf", "--id", "1"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			command := New()
			command.SetOut(&bytes.Buffer{})
			command.SetErr(&bytes.Buffer{})
			command.SetArgs(append([]string{"fetch"}, args...))
			err := command.Execute()
			var usage *exitcode.Error
			require.ErrorAs(t, err, &usage)
			assert.Equal(t, exitcode.Usage, usage.Code)
			assert.ErrorContains(t, err, "fetch peer")
		})
	}
}

func TestFetchPeerAllBadRefsIsUsageErrorWithRecords(t *testing.T) {
	var out bytes.Buffer
	command := New()
	command.SetOut(&out)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"fetch", "peer", "example", "123"})
	command.SilenceUsage = true // as the root command does

	err := command.Execute()
	var usage *exitcode.Error
	require.ErrorAs(t, err, &usage)
	assert.Equal(t, exitcode.Usage, usage.Code)

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	require.Len(t, lines, 2, "every bad ref still yields a record")
	assert.Contains(t, lines[0], `"ref":"example"`)
	assert.Contains(t, lines[0], `"code":"bad_ref"`)
	assert.Contains(t, lines[1], `"ref":"123"`)
}

func TestFetchPeerWithoutRefsIsUsageError(t *testing.T) {
	command := New()
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"fetch", "peer"})
	err := command.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires at least 1 arg")
}

func TestPeerOutcome(t *testing.T) {
	assert.NoError(t, peerOutcome(tgsvc.PeerStats{Fetched: 1, Failed: 3}), "one fetched record is success")
	assert.NoError(t, peerOutcome(tgsvc.PeerStats{}))

	err := peerOutcome(tgsvc.PeerStats{Failed: 2, BadRefs: 1})
	require.Error(t, err)
	assert.Equal(t, exitcode.Fail, exitcode.FromError(err))

	err = peerOutcome(tgsvc.PeerStats{Failed: 2, BadRefs: 2})
	assert.Equal(t, exitcode.Usage, exitcode.FromError(err))
}

func TestAnyPeerRef(t *testing.T) {
	assert.False(t, anyPeerRef([]string{"example", "123"}))
	assert.True(t, anyPeerRef([]string{"example", "https://t.me/+AbCd"}))
}
