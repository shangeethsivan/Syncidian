package config

import (
	"net"
	"strings"
)

// NormalizeAdminHost returns a hostname (no scheme, path, or port).
// Empty input stays empty so path-based /admin remains the default.
func NormalizeAdminHost(raw string) string {
	h := strings.TrimSpace(raw)
	if h == "" {
		return ""
	}
	h = strings.TrimPrefix(h, "https://")
	h = strings.TrimPrefix(h, "http://")
	if i := strings.IndexByte(h, '/'); i >= 0 {
		h = h[:i]
	}
	h = Hostname(h)
	if h == "" || h == "localhost" || net.ParseIP(h) != nil {
		return ""
	}
	return h
}

// NormalizeListenIP accepts a unicast IPv4 or IPv6 address. Wildcard and
// empty values are ignored so the server keeps its existing listen address.
func NormalizeListenIP(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	s = strings.Trim(s, "[]")
	ip := net.ParseIP(s)
	if ip == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLoopback() {
		return ""
	}
	return ip.String()
}

// Hostname strips a port and trailing dot from a Host header or URL host.
func Hostname(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimSuffix(host, ".")
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

// ListenPort returns the TCP port from Addr (default 8080).
func ListenPort(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "8080"
	}
	if strings.HasPrefix(addr, ":") {
		p := strings.TrimPrefix(addr, ":")
		if p == "" {
			return "8080"
		}
		return p
	}
	if _, port, err := net.SplitHostPort(addr); err == nil && port != "" {
		return port
	}
	return "8080"
}

func addrHost(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" || strings.HasPrefix(addr, ":") {
		return ""
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	return host
}

func isWildcardHost(host string) bool {
	return host == "" || host == "0.0.0.0" || host == "::" || host == "[::]"
}

// ListenAddrs returns the TCP addresses the process should bind.
//
// When AdminListenIP is unset, this is the single Addr (default :8080, all
// interfaces). When it is set, the process stops listening on 0.0.0.0 and
// binds loopback (for a tunnel or local reverse proxy) plus the Tailscale
// address, so the operator hostname is not reachable on the public internet.
func (c Config) ListenAddrs() []string {
	port := ListenPort(c.Addr)
	ip := strings.TrimSpace(c.AdminListenIP)
	if ip == "" {
		if strings.TrimSpace(c.Addr) == "" {
			return []string{":8080"}
		}
		return []string{c.Addr}
	}
	admin := net.JoinHostPort(ip, port)
	host := addrHost(c.Addr)
	if !isWildcardHost(host) {
		if host == ip || host == "["+ip+"]" {
			return []string{c.Addr}
		}
		explicit := c.Addr
		if explicit == admin {
			return []string{admin}
		}
		return []string{explicit, admin}
	}
	loopback := net.JoinHostPort("127.0.0.1", port)
	if loopback == admin {
		return []string{admin}
	}
	return []string{loopback, admin}
}
