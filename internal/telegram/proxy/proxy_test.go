package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromEnv_Unset(t *testing.T) {
	for _, k := range []string{
		"INDEXIT_PROXY_URL", "INDEXIT_PROXY_TYPE", "INDEXIT_PROXY_HOST",
		"INDEXIT_PROXY_PORT", "INDEXIT_PROXY_USER", "INDEXIT_PROXY_PASS",
		"INDEXIT_PROXY_SECRET",
	} {
		t.Setenv(k, "")
	}
	d, err := FromEnv()
	require.NoError(t, err)
	assert.Nil(t, d)
}

func TestFromEnv_DiscreteSOCKS5(t *testing.T) {
	t.Setenv("INDEXIT_PROXY_URL", "")
	t.Setenv("INDEXIT_PROXY_TYPE", "socks5")
	t.Setenv("INDEXIT_PROXY_HOST", "127.0.0.1")
	t.Setenv("INDEXIT_PROXY_PORT", "1080")
	t.Setenv("INDEXIT_PROXY_USER", "u")
	t.Setenv("INDEXIT_PROXY_PASS", "p")
	t.Setenv("INDEXIT_PROXY_SECRET", "")

	d, err := FromEnv()
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, TypeSOCKS5, d.Type)
	assert.Equal(t, "127.0.0.1", d.Host)
	assert.Equal(t, 1080, d.Port)
	assert.Equal(t, "u", d.User)
	assert.Equal(t, "p", d.Pass)
	assert.Equal(t, "socks5 127.0.0.1:1080", d.Display())
}

func TestFromEnv_URLSOCKS5(t *testing.T) {
	t.Setenv("INDEXIT_PROXY_URL", "socks5://alice:s%40cret@proxy.local:1080")
	d, err := FromEnv()
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, TypeSOCKS5, d.Type)
	assert.Equal(t, "proxy.local", d.Host)
	assert.Equal(t, 1080, d.Port)
	assert.Equal(t, "alice", d.User)
	assert.Equal(t, "s@cret", d.Pass)
}

func TestFromEnv_URLMTProto(t *testing.T) {
	t.Setenv("INDEXIT_PROXY_URL", "mtproto://mt.example:443?secret=dd0102030405060708090a0b0c0d0e0f10")
	d, err := FromEnv()
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, TypeMTProto, d.Type)
	assert.Equal(t, "mt.example", d.Host)
	assert.Equal(t, 443, d.Port)
	assert.Equal(t, "dd0102030405060708090a0b0c0d0e0f10", d.Secret)
}

func TestFromEnv_DiscreteMTProtoRequiresSecret(t *testing.T) {
	t.Setenv("INDEXIT_PROXY_URL", "")
	t.Setenv("INDEXIT_PROXY_TYPE", "mtproto")
	t.Setenv("INDEXIT_PROXY_HOST", "mt.example")
	t.Setenv("INDEXIT_PROXY_PORT", "443")
	t.Setenv("INDEXIT_PROXY_SECRET", "")
	_, err := FromEnv()
	assert.ErrorContains(t, err, "SECRET")
}

func TestFromEnv_DiscreteMTProtoNonHexFails(t *testing.T) {
	t.Setenv("INDEXIT_PROXY_URL", "")
	t.Setenv("INDEXIT_PROXY_TYPE", "mtproto")
	t.Setenv("INDEXIT_PROXY_HOST", "mt.example")
	t.Setenv("INDEXIT_PROXY_PORT", "443")
	t.Setenv("INDEXIT_PROXY_SECRET", "not-hex")
	_, err := FromEnv()
	assert.ErrorContains(t, err, "hex")
}

func TestFromEnv_PortOutOfRange(t *testing.T) {
	t.Setenv("INDEXIT_PROXY_URL", "")
	t.Setenv("INDEXIT_PROXY_TYPE", "socks5")
	t.Setenv("INDEXIT_PROXY_HOST", "host")
	t.Setenv("INDEXIT_PROXY_PORT", "70000")
	_, err := FromEnv()
	assert.ErrorContains(t, err, "out of range")
}

func TestFromEnv_TypeRequiredWhenAnySet(t *testing.T) {
	t.Setenv("INDEXIT_PROXY_URL", "")
	t.Setenv("INDEXIT_PROXY_TYPE", "")
	t.Setenv("INDEXIT_PROXY_HOST", "host")
	t.Setenv("INDEXIT_PROXY_PORT", "1080")
	_, err := FromEnv()
	assert.ErrorContains(t, err, "INDEXIT_PROXY_TYPE")
}

func TestFromEnv_UnknownType(t *testing.T) {
	t.Setenv("INDEXIT_PROXY_URL", "")
	t.Setenv("INDEXIT_PROXY_TYPE", "weird")
	t.Setenv("INDEXIT_PROXY_HOST", "h")
	t.Setenv("INDEXIT_PROXY_PORT", "1080")
	_, err := FromEnv()
	assert.ErrorContains(t, err, "unsupported")
}

func TestDescriptor_ResolverSOCKS5(t *testing.T) {
	d := &Descriptor{Type: TypeSOCKS5, Host: "127.0.0.1", Port: 1080}
	r, err := d.Resolver()
	require.NoError(t, err)
	assert.NotNil(t, r)
}

func TestDescriptor_ResolverHTTP(t *testing.T) {
	d := &Descriptor{Type: TypeHTTP, Host: "127.0.0.1", Port: 3128}
	r, err := d.Resolver()
	require.NoError(t, err)
	assert.NotNil(t, r)
}

func TestDescriptor_ResolverMTProto(t *testing.T) {
	d := &Descriptor{Type: TypeMTProto, Host: "mt", Port: 443, Secret: "dd0102030405060708090a0b0c0d0e0f10"}
	r, err := d.Resolver()
	require.NoError(t, err)
	assert.NotNil(t, r)
}

func TestFromEnv_DiscreteMTProtoBase64Secret(t *testing.T) {
	t.Setenv("INDEXIT_PROXY_URL", "")
	t.Setenv("INDEXIT_PROXY_TYPE", "mtproto")
	t.Setenv("INDEXIT_PROXY_HOST", "mt.example")
	t.Setenv("INDEXIT_PROXY_PORT", "443")
	t.Setenv("INDEXIT_PROXY_SECRET", "7gECAwQFBgcICQoLDA0ODxBleGFtcGxlLmNvbQ")

	d, err := FromEnv()
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, TypeMTProto, d.Type)

	r, err := d.Resolver()
	require.NoError(t, err)
	assert.NotNil(t, r)
}

func TestFromEnv_LinkMTProto(t *testing.T) {
	t.Setenv("INDEXIT_PROXY_URL",
		"tg://proxy?server=mt.example&port=443&secret=7gECAwQFBgcICQoLDA0ODxBleGFtcGxlLmNvbQ")

	d, err := FromEnv()
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, TypeMTProto, d.Type)
	assert.Equal(t, "mt.example", d.Host)
	assert.Equal(t, 443, d.Port)
	assert.Equal(t, "mtproto mt.example:443", d.Display())
}

func TestFromEnv_LinkMTProtoHTTPS(t *testing.T) {
	t.Setenv("INDEXIT_PROXY_URL",
		"https://t.me/proxy?server=mt.example&port=443&secret=dd0102030405060708090a0b0c0d0e0f10")

	d, err := FromEnv()
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, TypeMTProto, d.Type)
	assert.Equal(t, "mt.example", d.Host)
	assert.Equal(t, 443, d.Port)
}

func TestFromEnv_LinkSOCKS(t *testing.T) {
	t.Setenv("INDEXIT_PROXY_URL", "tg://socks?server=proxy.local&port=1080&user=alice&pass=s%40cret")

	d, err := FromEnv()
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, TypeSOCKS5, d.Type)
	assert.Equal(t, "proxy.local", d.Host)
	assert.Equal(t, 1080, d.Port)
	assert.Equal(t, "alice", d.User)
	assert.Equal(t, "s@cret", d.Pass)
}

func TestFromEnv_LinkRequiresServerAndPort(t *testing.T) {
	t.Setenv("INDEXIT_PROXY_URL", "tg://proxy?port=443&secret=dd0102030405060708090a0b0c0d0e0f10")
	_, err := FromEnv()
	assert.ErrorContains(t, err, "server=")

	t.Setenv("INDEXIT_PROXY_URL", "tg://proxy?server=mt.example&secret=dd0102030405060708090a0b0c0d0e0f10")
	_, err = FromEnv()
	assert.ErrorContains(t, err, "port=")
}

// A plain http:// URL stays an HTTP CONNECT proxy: only Telegram hosts are
// treated as proxy-sharing links.
func TestFromEnv_URLHTTPIsNotALink(t *testing.T) {
	t.Setenv("INDEXIT_PROXY_URL", "http://proxy.local:3128")
	d, err := FromEnv()
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, TypeHTTP, d.Type)
	assert.Equal(t, 3128, d.Port)
}

func TestDecodeSecret(t *testing.T) {
	tests := map[string]struct {
		secret string
		want   int // decoded length, 0 means error
	}{
		"simple hex":     {"000102030405060708090a0b0c0d0e0f", 16},
		"simple base64":  {"AAECAwQFBgcICQoLDA0ODw", 16},
		"secured hex":    {"dd0102030405060708090a0b0c0d0e0f10", 17},
		"faketls hex":    {"ee0102030405060708090a0b0c0d0e0f106578616d706c652e636f6d", 28},
		"faketls base64": {"7gECAwQFBgcICQoLDA0ODxBleGFtcGxlLmNvbQ", 28},
		"too short":      {"0001020304", 0},
		"unknown tag":    {"ab0102030405060708090a0b0c0d0e0f10", 0},
		"garbage":        {"not a secret!", 0},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := decodeSecret(tc.secret)
			if tc.want == 0 {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, got, tc.want)
		})
	}
}
