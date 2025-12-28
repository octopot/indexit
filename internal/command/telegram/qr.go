package telegram

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/gotd/td/telegram/auth/qrlogin"
	"rsc.io/qr"
)

// showQR returns a tgsvc.ShowQR that prints scanning instructions and renders
// the login token as a scannable QR code to w. It may be invoked repeatedly as
// the token is re-issued on expiry.
func showQR(w io.Writer) func(context.Context, qrlogin.Token) error {
	return func(_ context.Context, token qrlogin.Token) error {
		if _, err := fmt.Fprintln(w,
			"\nScan to log in — Telegram on your phone: Settings → Devices → Link Desktop Device"); err != nil {
			return err
		}
		if err := renderQR(w, token.URL()); err != nil {
			return err
		}
		_, err := fmt.Fprintf(w, "waiting for confirmation… (QR valid until %s, will refresh automatically)\n",
			token.Expires().Format("15:04:05"))
		return err
	}
}

// renderQR draws a QR for text using Unicode half-blocks on a forced
// white-on-black field, so the polarity stays scannable regardless of the
// terminal theme. Two QR rows are packed into each text line.
func renderQR(w io.Writer, text string) error {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return fmt.Errorf("encode qr: %w", err)
	}

	const border = 2 // quiet zone in modules
	black := func(x, y int) bool {
		if x < 0 || y < 0 || x >= code.Size || y >= code.Size {
			return false
		}
		return code.Black(x, y)
	}

	var b strings.Builder
	for y := -border; y < code.Size+border; y += 2 {
		b.WriteString("\x1b[107m\x1b[30m") // bright-white background, black foreground
		for x := -border; x < code.Size+border; x++ {
			top, bottom := black(x, y), black(x, y+1)
			switch {
			case top && bottom:
				b.WriteString("█") // full block
			case top:
				b.WriteString("▀") // upper half block
			case bottom:
				b.WriteString("▄") // lower half block
			default:
				b.WriteByte(' ')
			}
		}
		b.WriteString("\x1b[0m\n")
	}
	_, err = io.WriteString(w, b.String())
	return err
}
