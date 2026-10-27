package resolve

import "strings"

func MatchesHostname(hostname, pattern string) bool {
	hostname = normalizeHostname(hostname)
	pattern = normalizeHostname(pattern)
	if hostname == "" || pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	if !strings.HasPrefix(pattern, "*.") {
		return hostname == pattern
	}
	suffix := strings.TrimPrefix(pattern, "*.")
	if suffix == "" || !strings.HasSuffix(hostname, "."+suffix) {
		return false
	}
	prefix := strings.TrimSuffix(hostname, "."+suffix)
	return prefix != "" && !strings.Contains(prefix, ".")
}

func MatchesIstioHost(hostname, namespace, host string) bool {
	parts := strings.SplitN(host, "/", 2)
	if len(parts) == 2 {
		hostNamespace := parts[0]
		if hostNamespace != "*" && hostNamespace != "." && hostNamespace != namespace {
			return false
		}
		host = parts[1]
	}
	return MatchesHostname(hostname, host)
}

func HostnamesIntersect(first, second string) bool {
	first = normalizeHostname(first)
	second = normalizeHostname(second)
	if first == "" || first == "*" || second == "" || second == "*" {
		return true
	}
	firstWildcard := strings.HasPrefix(first, "*.")
	secondWildcard := strings.HasPrefix(second, "*.")
	switch {
	case !firstWildcard && !secondWildcard:
		return first == second
	case firstWildcard && !secondWildcard:
		return MatchesHostname(second, first)
	case !firstWildcard && secondWildcard:
		return MatchesHostname(first, second)
	default:
		return strings.TrimPrefix(first, "*.") == strings.TrimPrefix(second, "*.")
	}
}

func normalizeHostname(hostname string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(hostname)), ".")
}
