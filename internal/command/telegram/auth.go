package telegram

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	indexlog "go.octolab.org/toolset/indexit/internal/log"
	tgsvc "go.octolab.org/toolset/indexit/internal/telegram"
	tgproxy "go.octolab.org/toolset/indexit/internal/telegram/proxy"
)

func authCommand(opt *options) *cobra.Command {
	command := cobra.Command{
		Use:   "auth",
		Short: "Manage Telegram authentication",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	command.AddCommand(
		authLoginCommand(opt),
		authStatusCommand(opt),
		authLogoutCommand(opt),
	)
	return &command
}

// errSwitchToQR is returned by the code prompt when the user opts out of waiting
// for a phone code and chooses QR login instead. The command catches it and
// restarts with a QR-capable client (the code flow runs with updates disabled).
var errSwitchToQR = errors.New("switch to qr login")

func authLoginCommand(opt *options) *cobra.Command {
	var useQR bool
	command := &cobra.Command{
		Use:   "login",
		Short: "Run interactive Telegram login",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := pathsFromFlags(opt)
			if err != nil {
				return err
			}
			ctx, cancel, err := contextFromFlags(cmd, opt)
			if err != nil {
				return err
			}
			defer cancel()

			loginQR := func() error {
				client, err := newQRClient(cmd, paths)
				if err != nil {
					return err
				}
				prompt := newPromptAuth(cmd.InOrStdin(), cmd.ErrOrStderr())
				return tgsvc.LoginQR(ctx, client, showQR(cmd.ErrOrStderr()), prompt.Password)
			}
			if useQR {
				return loginQR()
			}

			client, err := newClient(cmd, paths)
			if err != nil {
				return err
			}
			prompt := newPromptAuth(cmd.InOrStdin(), cmd.ErrOrStderr())
			prompt.qrFallback = true
			if err := tgsvc.Login(ctx, client, prompt); err != nil {
				if errors.Is(err, errSwitchToQR) {
					fmt.Fprintln(cmd.ErrOrStderr(), "switching to QR login…")
					return loginQR()
				}
				return err
			}
			return nil
		},
	}
	command.Flags().BoolVar(&useQR, "qr", false,
		"log in by scanning a QR code from a logged-in device (no phone code needed)")
	return command
}

func authStatusCommand(opt *options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Print Telegram session status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := pathsFromFlags(opt)
			if err != nil {
				return err
			}
			if descriptor, derr := tgproxy.FromEnv(); derr != nil {
				return usageErr(derr)
			} else if descriptor != nil {
				indexlog.FromContext(cmd.Context()).Logger.Info("proxy",
					"type", string(descriptor.Type),
					"host", descriptor.Host,
					"port", descriptor.Port)
			}
			if _, err := os.Stat(paths.Session); os.IsNotExist(err) {
				return tgsvc.PrintStatus(cmd.OutOrStdout(), tgsvc.Status{SessionPath: paths.Session})
			} else if err != nil {
				return err
			}
			ctx, cancel, err := contextFromFlags(cmd, opt)
			if err != nil {
				return err
			}
			defer cancel()
			client, err := newClient(cmd, paths)
			if err != nil {
				return err
			}
			status, err := tgsvc.GetStatus(ctx, client, paths.Session)
			if err != nil {
				return err
			}
			return tgsvc.PrintStatus(cmd.OutOrStdout(), status)
		},
	}
}

func authLogoutCommand(opt *options) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Invalidate Telegram session and remove local session file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := pathsFromFlags(opt)
			if err != nil {
				return err
			}
			if _, err := os.Stat(paths.Session); os.IsNotExist(err) {
				return nil
			} else if err != nil {
				return err
			}
			ctx, cancel, err := contextFromFlags(cmd, opt)
			if err != nil {
				return err
			}
			defer cancel()
			client, err := newClient(cmd, paths)
			if err != nil {
				return err
			}
			return tgsvc.Logout(ctx, client, paths.Session)
		},
	}
}

type promptAuth struct {
	in         io.Reader
	out        io.Writer
	reader     *bufio.Reader
	client     *auth.Client
	phone      string
	qrFallback bool // offer QR login as an escape hatch from the code prompt
}

func newPromptAuth(in io.Reader, out io.Writer) *promptAuth {
	return &promptAuth{
		in:     in,
		out:    out,
		reader: bufio.NewReader(in),
	}
}

// AttachClient satisfies telegram.CodeResender so Code can resend the login code.
func (p *promptAuth) AttachClient(client *auth.Client) {
	p.client = client
}

func (p *promptAuth) Phone(ctx context.Context) (string, error) {
	phone, err := p.prompt("phone: ")
	if err == nil {
		p.phone = phone
	}
	return phone, err
}

func (p *promptAuth) Code(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	p.announceCode(sentCode)
	for {
		code, err := p.prompt("code: ")
		if err != nil {
			return "", err
		}
		if code != "" {
			if p.qrFallback && strings.EqualFold(code, "qr") {
				return "", errSwitchToQR
			}
			return code, nil
		}
		if p.canResend(sentCode) {
			if next := p.resendCode(ctx, sentCode); next != nil {
				sentCode = next
				p.announceCode(sentCode)
			}
			continue
		}
		fmt.Fprintln(p.out, "  no other delivery channel — Telegram won't resend this code.")
		if p.qrFallback {
			fmt.Fprintln(p.out, "  keep waiting, or type \"qr\" to log in from another device instead.")
		}
	}
}

// announceCode tells the user where Telegram actually delivered the code and how
// to switch channels, instead of leaving them guessing at a bare "code:" prompt.
func (p *promptAuth) announceCode(sentCode *tg.AuthSentCode) {
	fmt.Fprintf(p.out, "code sent via %s\n", describeSentCode(sentCode))
	if p.canResend(sentCode) {
		fmt.Fprintf(p.out,
			"  not arriving? submit an empty code (just Enter) to resend via %s\n",
			describeNextType(sentCode.NextType))
	}
	if p.qrFallback {
		fmt.Fprintln(p.out, "  or type \"qr\" to log in by scanning a code from another device")
	}
}

// canResend reports whether a resend can actually do something. An empty
// NextType means there is no next channel in the pool, so Telegram would just
// reject the resend — advertising it then only misleads.
func (p *promptAuth) canResend(sentCode *tg.AuthSentCode) bool {
	return p.client != nil && p.phone != "" && sentCode != nil &&
		sentCode.PhoneCodeHash != "" && sentCode.NextType != nil
}

func (p *promptAuth) resendCode(ctx context.Context, sentCode *tg.AuthSentCode) *tg.AuthSentCode {
	if !p.canResend(sentCode) {
		fmt.Fprintln(p.out, "  cannot resend automatically; re-run `auth login` to request a new code")
		return nil
	}
	sent, err := p.client.ResendCode(ctx, p.phone, sentCode.PhoneCodeHash)
	if err != nil {
		if tgerr.Is(err, "SEND_CODE_UNAVAILABLE") {
			fmt.Fprintf(p.out,
				"  Telegram refused to resend (SEND_CODE_UNAVAILABLE): no alternative channel is\n"+
					"  available. The code can only arrive in the \"Telegram\" service chat (id 777000,\n"+
					"  shown in-app as +42777) of %s. If it is silent there, make sure that chat is\n"+
					"  not blocked (Settings > Privacy > Blocked Users), or use `auth login --qr` to\n"+
					"  log in from another device without a code.\n",
				p.phone)
			return nil
		}
		fmt.Fprintf(p.out, "  resend failed: %v\n", err)
		return nil
	}
	next, ok := sent.(*tg.AuthSentCode)
	if !ok {
		fmt.Fprintf(p.out, "  resend returned unexpected %T\n", sent)
		return nil
	}
	return next
}

// describeSentCode renders tg.AuthSentCode.Type into a human destination so the
// user knows whether to look in the Telegram app, SMS, a call, etc.
func describeSentCode(sentCode *tg.AuthSentCode) string {
	if sentCode == nil {
		return "an unknown channel"
	}
	switch t := sentCode.Type.(type) {
	case *tg.AuthSentCodeTypeApp:
		return fmt.Sprintf("the Telegram app — check the \"Telegram\" service chat (id 777000, shown in-app as +42777); %d digits", t.Length)
	case *tg.AuthSentCodeTypeSMS:
		return fmt.Sprintf("SMS; %d digits", t.Length)
	case *tg.AuthSentCodeTypeCall:
		return fmt.Sprintf("a voice call; %d digits", t.Length)
	case *tg.AuthSentCodeTypeMissedCall:
		return fmt.Sprintf("a missed call (code is the caller's last digits); %d digits", t.Length)
	case *tg.AuthSentCodeTypeFragmentSMS:
		return fmt.Sprintf("Fragment (%s); %d digits", t.URL, t.Length)
	case *tg.AuthSentCodeTypeEmailCode:
		return fmt.Sprintf("email %s; %d digits", t.EmailPattern, t.Length)
	default:
		return sentCode.Type.TypeName()
	}
}

func describeNextType(next tg.AuthCodeTypeClass) string {
	switch next.(type) {
	case *tg.AuthCodeTypeSMS:
		return "SMS"
	case *tg.AuthCodeTypeCall:
		return "a voice call"
	case *tg.AuthCodeTypeFlashCall:
		return "a flash call"
	case *tg.AuthCodeTypeMissedCall:
		return "a missed call"
	case *tg.AuthCodeTypeFragmentSMS:
		return "Fragment"
	case nil:
		return ""
	default:
		return "another channel"
	}
}

func (p *promptAuth) Password(ctx context.Context) (string, error) {
	if file, ok := p.in.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		if _, err := fmt.Fprint(p.out, "2FA password: "); err != nil {
			return "", err
		}
		password, err := term.ReadPassword(int(file.Fd()))
		if _, printErr := fmt.Fprintln(p.out); printErr != nil && err == nil {
			err = printErr
		}
		return string(password), err
	}
	return p.prompt("2FA password: ")
}

func (p *promptAuth) AcceptTermsOfService(ctx context.Context, tos tg.HelpTermsOfService) error {
	return fmt.Errorf("sign-up and terms-of-service acceptance are out of scope for this PoC")
}

func (p *promptAuth) SignUp(ctx context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("sign-up is out of scope for this PoC")
}

func (p *promptAuth) prompt(label string) (string, error) {
	if _, err := fmt.Fprint(p.out, label); err != nil {
		return "", err
	}
	line, err := p.reader.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
