package token

import "net"

// normCIDRs accepts single IPs or CIDR blocks and returns canonical CIDRs.
func normCIDRs(in []string) ([]string, bool) {
	out := []string{}
	for _, s := range in {
		if s == "" {
			continue
		}
		if !containsSlash(s) {
			ip := net.ParseIP(s)
			if ip == nil {
				return nil, false
			}
			if ip.To4() != nil {
				s += "/32"
			} else {
				s += "/128"
			}
		}
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			return nil, false
		}
		out = append(out, n.String())
	}
	return out, len(out) <= 20
}

func containsSlash(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return true
		}
	}
	return false
}

func ipAllowed(cidrs []string, ip string) bool {
	if len(cidrs) == 0 {
		return true
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, c := range cidrs {
		if _, n, err := net.ParseCIDR(c); err == nil && n.Contains(parsed) {
			return true
		}
	}
	return false
}
