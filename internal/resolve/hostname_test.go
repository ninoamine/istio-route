package resolve

import "testing"

func TestMatchesHostname(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		pattern  string
		want     bool
	}{
		{name: "exact hostname", hostname: "shop.example.com", pattern: "shop.example.com", want: true},
		{name: "case and trailing dot normalize", hostname: "SHOP.EXAMPLE.COM.", pattern: "shop.example.com", want: true},
		{name: "wildcard matches one label", hostname: "shop.example.com", pattern: "*.example.com", want: true},
		{name: "wildcard does not match multiple labels", hostname: "shop.eu.example.com", pattern: "*.example.com"},
		{name: "wildcard does not match apex", hostname: "example.com", pattern: "*.example.com"},
		{name: "non matching exact", hostname: "api.example.com", pattern: "shop.example.com"},
		{name: "empty hostname", hostname: "", pattern: "*.example.com"},
		{name: "empty pattern", hostname: "shop.example.com", pattern: ""},
		{name: "wildcard suffix matches normalized hostname", hostname: "shop.example.com.", pattern: "*.EXAMPLE.COM.", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchesHostname(tt.hostname, tt.pattern); got != tt.want {
				t.Fatalf("MatchesHostname(%q, %q) = %t, want %t", tt.hostname, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestMatchesIstioHost(t *testing.T) {
	tests := []struct {
		name      string
		hostname  string
		namespace string
		host      string
		want      bool
	}{
		{name: "unqualified host matches", hostname: "shop.example.com", namespace: "shop", host: "shop.example.com", want: true},
		{name: "qualified matching namespace", hostname: "shop.example.com", namespace: "shop", host: "shop/shop.example.com", want: true},
		{name: "qualified other namespace does not match", hostname: "shop.example.com", namespace: "shop", host: "infra/shop.example.com"},
		{name: "wildcard namespace qualifier", hostname: "shop.example.com", namespace: "shop", host: "*/shop.example.com", want: true},
		{name: "current namespace qualifier", hostname: "shop.example.com", namespace: "shop", host: "./shop.example.com", want: true},
		{name: "namespace qualifier without host", hostname: "shop.example.com", namespace: "shop", host: "shop/"},
		{name: "wildcard host with namespace", hostname: "api.example.com", namespace: "shop", host: "shop/*.example.com", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchesIstioHost(tt.hostname, tt.namespace, tt.host); got != tt.want {
				t.Fatalf("MatchesIstioHost(%q, %q, %q) = %t, want %t", tt.hostname, tt.namespace, tt.host, got, tt.want)
			}
		})
	}
}

func TestHostnamesIntersect(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{name: "same exact", a: "shop.example.com", b: "shop.example.com", want: true},
		{name: "listener wildcard intersects exact route", a: "*.example.com", b: "shop.example.com", want: true},
		{name: "route wildcard intersects exact listener", a: "shop.example.com", b: "*.example.com", want: true},
		{name: "same wildcard", a: "*.example.com", b: "*.example.com", want: true},
		{name: "overlapping wildcards", a: "*.example.com", b: "*.shop.example.com"},
		{name: "different exact names", a: "shop.example.com", b: "api.example.com"},
		{name: "empty hostname is universal", a: "", b: "shop.example.com", want: true},
		{name: "empty listener is universal", a: "", b: "*.example.com", want: true},
		{name: "wildcard listener is universal", a: "*", b: "shop.example.com", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HostnamesIntersect(tt.a, tt.b); got != tt.want {
				t.Fatalf("HostnamesIntersect(%q, %q) = %t, want %t", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
