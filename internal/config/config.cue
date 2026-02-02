// Schema for the optional indexit config file,
// $XDG_CONFIG_HOME/indexit/config.toml (~/.config/indexit/config.toml without
// XDG_CONFIG_HOME). The CLI and the service read the same file; a key outside
// this schema is an error. Check a copy with
// `cue vet -c internal/config/config.cue config.toml`; `make config-vet` checks
// the fixtures in internal/config/testdata, which internal/config also reads.
package config

#Config: {
	// The local image service.
	service?: #Service
}

#Service: {
	// Scheme, host and optional port of the photograph URLs written into
	// exports: http or https, a host name or an IPv4 address, a port from 1
	// to 65535 and nothing after them but an optional slash. Exports keep
	// the origin they were written with. Default:
	// "http://indexit.localhost:17840", where `indexit service run` listens.
	origin?: string & =~"^https?://[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)*(:([1-9][0-9]{0,3}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5]))?/?$"

	// The statistics page at /: photographs saved, their size, the latest
	// ones and what the service served. false leaves / a plain-text health
	// line. Default: true when origin is local (localhost, a .localhost name
	// or a loopback IP address), false otherwise: another origin means a
	// proxy in front of the service, which would expose the page.
	dashboard?: bool

	// The directory of the saved photographs: an absolute path, or one in the
	// home directory starting with ~/. It can be a folder another computer
	// shares, such as one in Google Drive; the locks and the request counts
	// stay in the data directory. Default: images in the data directory.
	images?: string & =~"^(/|~/)."
}

#Config
