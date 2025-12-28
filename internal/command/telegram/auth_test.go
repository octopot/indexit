package telegram

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gotd/td/tg"
)

func TestCodePromptQRFallback(t *testing.T) {
	sentCode := &tg.AuthSentCode{Type: &tg.AuthSentCodeTypeApp{Length: 5}}

	t.Run("qr switches when fallback enabled", func(t *testing.T) {
		p := newPromptAuth(strings.NewReader("qr\n"), &bytes.Buffer{})
		p.qrFallback = true
		if _, err := p.Code(context.Background(), sentCode); !errors.Is(err, errSwitchToQR) {
			t.Fatalf("want errSwitchToQR, got %v", err)
		}
	})

	t.Run("qr is a literal code when fallback disabled", func(t *testing.T) {
		p := newPromptAuth(strings.NewReader("qr\n"), &bytes.Buffer{})
		code, err := p.Code(context.Background(), sentCode)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if code != "qr" {
			t.Fatalf("want literal %q, got %q", "qr", code)
		}
	})

	t.Run("a real code is returned verbatim", func(t *testing.T) {
		p := newPromptAuth(strings.NewReader("12345\n"), &bytes.Buffer{})
		p.qrFallback = true
		code, err := p.Code(context.Background(), sentCode)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if code != "12345" {
			t.Fatalf("want %q, got %q", "12345", code)
		}
	})
}
