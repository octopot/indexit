package uid

import "testing"

// FuzzParse asserts that the parser cannot panic on arbitrary input and that
// any PeerRef it returns can be rendered via String() without panicking.
//
// The seed corpus covers the grammar branches enumerated in plan §4.1 so the
// engine starts from realistic shapes before mutating.
func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		"",
		"   ",
		"@telegram",
		"@beta",
		"@abc",
		"user:@kamilsk",
		"user:42",
		"chat:42",
		"-42",
		"channel:42",
		"-10042",
		"channel:42:7",
		"-10042:7",
		"https://t.me/telegram",
		"https://t.me/telegram/123",
		"https://t.me/telegram/7/123",
		"https://t.me/c/77/123",
		"https://t.me/c/77/7/123",
		"http://t.me/foo",
		"https://t.me/s/telegram",
		"https://t.me/s/telegram/123",
		"@",
		"::",
		":",
		"user:",
		"channel:",
		"channel:abc",
		"channel:-1",
		"channel:42:",
		"channel:42:abc",
		"-100",
		"-",
		"https://t.me/",
		"https://t.me/c/",
		"https://t.me/c/x/y",
		"https://example.com/foo",
		"1FOO",
		"\x00\x01\x02",
		"\n\t\r",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		ref, err := Parse(input)
		if err != nil {
			return // rejections are fine — the contract is "no panic"
		}
		_ = ref.String()
	})
}
