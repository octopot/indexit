package telegram

import (
	"bytes"
	"strings"
	"testing"

	"rsc.io/qr"
)

// TestRenderQR reconstructs the module grid from renderQR's half-block output
// and checks it against the source bitmap. This guards the polarity (dark
// modules must stay dark) and the two-rows-per-line packing — a flipped or
// shifted mapping would yield an unscannable code.
func TestRenderQR(t *testing.T) {
	const sample = "tg://login?token=AQID_sample_TOKEN-0123456789abcdef"
	const border = 2 // must match renderQR

	code, err := qr.Encode(sample, qr.M)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	var buf bytes.Buffer
	if err := renderQR(&buf, sample); err != nil {
		t.Fatalf("renderQR: %v", err)
	}

	want := func(x, y int) bool {
		if x < 0 || y < 0 || x >= code.Size || y >= code.Size {
			return false
		}
		return code.Black(x, y)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	for row, line := range lines {
		line = strings.TrimPrefix(line, "\x1b[107m\x1b[30m")
		line = strings.TrimSuffix(line, "\x1b[0m")
		y := -border + row*2
		for col, r := range []rune(line) {
			x := -border + col
			var top, bottom bool
			switch r {
			case '█':
				top, bottom = true, true
			case '▀':
				top = true
			case '▄':
				bottom = true
			case ' ':
			default:
				t.Fatalf("row %d col %d: unexpected rune %q", row, col, r)
			}
			if top != want(x, y) || bottom != want(x, y+1) {
				t.Fatalf("module mismatch at (%d,%d): got top=%v bottom=%v, want top=%v bottom=%v",
					x, y, top, bottom, want(x, y), want(x, y+1))
			}
		}
	}
}
