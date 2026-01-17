package skill_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"

	. "go.octolab.org/toolset/indexit/internal/skill"
)

func skillFS(text string) fstest.MapFS {
	return fstest.MapFS{
		"SKILL.md":                {Data: []byte(text)},
		"references/workflows.md": {Data: []byte("Back to [the skill](../SKILL.md#indexit).\n")},
	}
}

func TestCheck(t *testing.T) {
	assert.Empty(t, Check("indexit", skillFS(skillMD)))

	bad := strings.NewReplacer(
		"name: indexit", "name: other",
		`version: "0.2.0"`, `version: "0.3.0"`,
		"references/workflows.md", "references/missing.md",
	).Replace(skillMD)
	var problems []string
	for _, err := range Check("indexit", skillFS(bad)) {
		problems = append(problems, err.Error())
	}
	joined := strings.Join(problems, "\n")
	assert.Contains(t, joined, "name: must be indexit")
	assert.Contains(t, joined, "tool-version-range: must start at the release")
	assert.Contains(t, joined, "broken link: references/missing.md")

	outside := skillFS(strings.Replace(skillMD, "references/workflows.md", "../README.md", 1))
	assert.ErrorContains(t, Check("indexit", outside)[0], "link leaves the skill")
	assert.ErrorContains(t, Check("indexit", fstest.MapFS{"README.md": {}})[0], "SKILL.md: missing")
}
