package handler

import (
	"os"
	"testing"

	"openai-backend/internal/service"
)

func TestEnvKeySuffixFromOwnedBy_Normalization(t *testing.T) {
	cases := map[string]string{
		"volcano":      "VOLCANO",
		"MiniMax":      "MINIMAX",
		"mini-max":     "MINI_MAX",
		"  deep seek ": "DEEP_SEEK",
		"":             "",
	}
	for in, want := range cases {
		got := service.EnvKeySuffixFromOwnedBy(in)
		if got != want {
			t.Fatalf("EnvKeySuffixFromOwnedBy(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseUpstreamFallbacks_ParsesAndNormalizes(t *testing.T) {
	old, had := os.LookupEnv("UPSTREAM_FALLBACKS")
	_ = os.Setenv("UPSTREAM_FALLBACKS", "volcano=minimax;minimax=volcano, minimax ;badpair;=x;openai=")
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("UPSTREAM_FALLBACKS", old)
		} else {
			_ = os.Unsetenv("UPSTREAM_FALLBACKS")
		}
	})

	m := service.ParseUpstreamFallbacksFromEnv()
	if len(m) == 0 {
		t.Fatalf("expected non-empty fallback map")
	}

	v, ok := m["VOLCANO"]
	if !ok || len(v) != 1 || v[0] != "MINIMAX" {
		t.Fatalf("VOLCANO fallbacks = %#v, want [MINIMAX]", v)
	}

	mm, ok := m["MINIMAX"]
	if !ok || len(mm) != 1 || mm[0] != "VOLCANO" {
		t.Fatalf("MINIMAX fallbacks = %#v, want [VOLCANO]", mm)
	}
}

func TestResolveUpstreamForProvider_PicksDeterministicKey(t *testing.T) {
	// Set provider-specific base URL and multiple keys.
	oldBase, hadBase := os.LookupEnv("UPSTREAM_VOLCANO_BASE_URL")
	oldKeys, hadKeys := os.LookupEnv("UPSTREAM_VOLCANO_API_KEYS")
	_ = os.Setenv("UPSTREAM_VOLCANO_BASE_URL", "https://example.invalid/v1")
	_ = os.Setenv("UPSTREAM_VOLCANO_API_KEYS", "k1,k2,k3")
	// Ensure global fallback doesn't interfere.
	oldGlobalBase, hadGlobalBase := os.LookupEnv("UPSTREAM_BASE_URL")
	oldGlobalKey, hadGlobalKey := os.LookupEnv("UPSTREAM_API_KEY")
	_ = os.Unsetenv("UPSTREAM_BASE_URL")
	_ = os.Unsetenv("UPSTREAM_API_KEY")

	t.Cleanup(func() {
		if hadBase {
			_ = os.Setenv("UPSTREAM_VOLCANO_BASE_URL", oldBase)
		} else {
			_ = os.Unsetenv("UPSTREAM_VOLCANO_BASE_URL")
		}
		if hadKeys {
			_ = os.Setenv("UPSTREAM_VOLCANO_API_KEYS", oldKeys)
		} else {
			_ = os.Unsetenv("UPSTREAM_VOLCANO_API_KEYS")
		}
		if hadGlobalBase {
			_ = os.Setenv("UPSTREAM_BASE_URL", oldGlobalBase)
		} else {
			_ = os.Unsetenv("UPSTREAM_BASE_URL")
		}
		if hadGlobalKey {
			_ = os.Setenv("UPSTREAM_API_KEY", oldGlobalKey)
		} else {
			_ = os.Unsetenv("UPSTREAM_API_KEY")
		}
	})

	seed := "chatcmpl-test-seed"
	up1 := service.ResolveUpstreamForProvider("VOLCANO", seed)
	up2 := service.ResolveUpstreamForProvider("VOLCANO", seed)
	if up1.BaseURL != "https://example.invalid/v1" {
		t.Fatalf("baseURL = %q, want %q", up1.BaseURL, "https://example.invalid/v1")
	}
	if up1.APIKey == "" {
		t.Fatalf("apiKey should not be empty")
	}
	if up1.APIKey != up2.APIKey {
		t.Fatalf("apiKey should be deterministic for same seed: %q vs %q", up1.APIKey, up2.APIKey)
	}
}

func TestShouldFallbackForStatus(t *testing.T) {
	cases := map[int]bool{
		200: false,
		400: false,
		401: true,
		403: true,
		429: true,
		500: true,
		503: true,
	}

	for statusCode, want := range cases {
		got := service.ShouldFallbackForStatus(statusCode)
		if got != want {
			t.Fatalf("ShouldFallbackForStatus(%d) = %v, want %v", statusCode, got, want)
		}
	}
}
