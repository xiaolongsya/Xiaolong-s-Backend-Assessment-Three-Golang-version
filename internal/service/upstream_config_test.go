package service

import (
	"os"
	"testing"
)

func TestParseCommaListAndEnvKeyHelpers(t *testing.T) {
	if got := parseCommaList(" a, b ,, c "); len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("parseCommaList = %#v", got)
	}
	if got := EnvKeySuffixFromOwnedBy(" mini-max "); got != "MINI_MAX" {
		t.Fatalf("EnvKeySuffixFromOwnedBy = %q", got)
	}
	if got := pickKeyDeterministic([]string{"k1"}, "seed"); got != "k1" {
		t.Fatalf("pickKeyDeterministic single = %q", got)
	}
	if got := pickKeyDeterministic(nil, "seed"); got != "" {
		t.Fatalf("pickKeyDeterministic nil = %q", got)
	}
}

func TestResolveUpstreamForProviderAndFallbackStatus(t *testing.T) {
	oldBase, hadBase := os.LookupEnv("UPSTREAM_BASE_URL")
	oldKey, hadKey := os.LookupEnv("UPSTREAM_API_KEY")
	oldProviderBase, hadProviderBase := os.LookupEnv("UPSTREAM_VOLCANO_BASE_URL")
	oldProviderKey, hadProviderKey := os.LookupEnv("UPSTREAM_VOLCANO_API_KEY")
	t.Cleanup(func() {
		if hadBase {
			_ = os.Setenv("UPSTREAM_BASE_URL", oldBase)
		} else {
			_ = os.Unsetenv("UPSTREAM_BASE_URL")
		}
		if hadKey {
			_ = os.Setenv("UPSTREAM_API_KEY", oldKey)
		} else {
			_ = os.Unsetenv("UPSTREAM_API_KEY")
		}
		if hadProviderBase {
			_ = os.Setenv("UPSTREAM_VOLCANO_BASE_URL", oldProviderBase)
		} else {
			_ = os.Unsetenv("UPSTREAM_VOLCANO_BASE_URL")
		}
		if hadProviderKey {
			_ = os.Setenv("UPSTREAM_VOLCANO_API_KEY", oldProviderKey)
		} else {
			_ = os.Unsetenv("UPSTREAM_VOLCANO_API_KEY")
		}
	})

	_ = os.Setenv("UPSTREAM_BASE_URL", "https://global.example/v1/")
	_ = os.Setenv("UPSTREAM_API_KEY", "global-key")
	_ = os.Setenv("UPSTREAM_VOLCANO_BASE_URL", "https://volcano.example/v1/")
	_ = os.Setenv("UPSTREAM_VOLCANO_API_KEY", "provider-key")

	global := ResolveUpstreamForProvider("", "seed")
	if global.BaseURL != "https://global.example/v1" || global.APIKey != "global-key" {
		t.Fatalf("global = %#v", global)
	}
	provider := ResolveUpstreamForProvider("VOLCANO", "seed")
	if provider.BaseURL != "https://volcano.example/v1" || provider.APIKey != "provider-key" {
		t.Fatalf("provider = %#v", provider)
	}

	if !ShouldFallbackForStatus(500) || !ShouldFallbackForStatus(429) || ShouldFallbackForStatus(400) {
		t.Fatalf("ShouldFallbackForStatus not matching expectations")
	}
}
