package telegram

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.octolab.org/toolset/indexit/internal/exitcode"
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
