package curl

import "github.com/rumpl/gash/internal/commandhelp"

var helpInfo = commandhelp.Info{
	Name:    "curl",
	Summary: "transfer a URL using an explicit gash network policy",
	Usage:   "curl [OPTIONS] URL",
	Description: []string{
		"Opt-in in-process HTTP(S) client inspired by just-bash curl. The command is registered only when gash.Options.Network is set or the CLI --network-allow flag is used.",
	},
	Options: []string{
		"-X, --request METHOD       use METHOD for the request",
		"-H, --header HEADER        add a request header",
		"-A, --user-agent VALUE     set User-Agent",
		"-u, --user USER[:PASS]     send HTTP Basic authentication",
		"-d, --data DATA            send form-encoded request body (@FILE reads virtual file)",
		"-F, --form NAME=VALUE      send multipart form data (@FILE uploads virtual file)",
		"-T, --upload-file FILE     upload virtual FILE with PUT (or selected method)",
		"-b, --cookie DATA|FILE     send cookie data or read it from a virtual file",
		"-I, --head                 fetch headers only",
		"-i, --include              include response headers in output",
		"-f, --fail                 fail on HTTP status >= 400",
		"-L, --location             follow redirects; every redirect is policy-checked",
		"-s, --silent               suppress errors",
		"-S, --show-error           show errors even with --silent",
		"-v, --verbose              write request/response diagnostics to stderr",
		"-w, --write-out FORMAT     print selected variables after the response",
		"-o, --output FILE          write response to a virtual file",
		"--max-time SECONDS         cap request duration",
		"--connect-timeout SECONDS  accepted as a duration hint",
		"    --help                 display this help and exit",
	},
	Notes: []string{
		"Network is unavailable by default. Policy allow-lists scheme, host, port, path prefix, methods, and request headers; URL credentials are rejected.",
		"DNS is resolved through the policy resolver and public-address firewall by default; loopback, private, link-local, multicast, and unspecified addresses are denied unless AllowPrivateIPs is explicitly set.",
		"Deferred differences: progress meters, netrc, proxy support, TLS/client certificates, cookie-jar persistence, compressed transfer decoding controls, many curl --write-out variables, and exact upstream diagnostic wording.",
	},
}
