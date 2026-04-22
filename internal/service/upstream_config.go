package service

import (
	"hash/fnv"
	"net/http"
	"os"
	"strings"
)

type UpstreamConfig struct {
	BaseURL string
	APIKey  string
}

func EnvKeySuffixFromOwnedBy(ownedBy string) string {
	s := strings.TrimSpace(ownedBy)
	if s == "" {
		return ""
	}
	s = strings.ToUpper(s)
	var b strings.Builder
	b.Grow(len(s))
	lastUnderscore := false
	for _, r := range s {
		isAZ := r >= 'A' && r <= 'Z'
		is09 := r >= '0' && r <= '9'
		if isAZ || is09 {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_")
	return out
}

func parseCommaList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func pickKeyDeterministic(keys []string, seed string) string {
	if len(keys) == 0 {
		return ""
	}
	if len(keys) == 1 {
		return keys[0]
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	idx := int(h.Sum32() % uint32(len(keys)))
	return keys[idx]
}

func ParseUpstreamFallbacksFromEnv() map[string][]string {
	raw := strings.TrimSpace(os.Getenv("UPSTREAM_FALLBACKS"))
	if raw == "" {
		return map[string][]string{}
	}

	out := make(map[string][]string)
	pairs := strings.Split(raw, ";")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			continue
		}
		primary := EnvKeySuffixFromOwnedBy(kv[0])
		if primary == "" {
			continue
		}
		rawList := strings.TrimSpace(kv[1])
		if rawList == "" {
			continue
		}
		items := strings.Split(rawList, ",")
		fallbacks := make([]string, 0, len(items))
		for _, it := range items {
			s := EnvKeySuffixFromOwnedBy(it)
			if s != "" && s != primary {
				fallbacks = append(fallbacks, s)
			}
		}
		if len(fallbacks) > 0 {
			out[primary] = fallbacks
		}
	}
	return out
}

func ResolveUpstreamForProvider(providerSuffix string, completionID string) UpstreamConfig {
	provider := strings.TrimSpace(providerSuffix)

	baseURL := ""
	if provider != "" {
		baseURL = strings.TrimSpace(os.Getenv("UPSTREAM_" + provider + "_BASE_URL"))
	}
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("UPSTREAM_BASE_URL"))
	}
	baseURL = strings.TrimRight(baseURL, "/")

	var keys []string
	if provider != "" {
		keys = parseCommaList(os.Getenv("UPSTREAM_" + provider + "_API_KEYS"))
		if len(keys) == 0 {
			k := strings.TrimSpace(os.Getenv("UPSTREAM_" + provider + "_API_KEY"))
			if k != "" {
				keys = []string{k}
			}
		}
	}
	if len(keys) == 0 {
		k := strings.TrimSpace(os.Getenv("UPSTREAM_API_KEY"))
		if k != "" {
			keys = []string{k}
		}
	}

	apiKey := pickKeyDeterministic(keys, completionID)
	return UpstreamConfig{BaseURL: baseURL, APIKey: apiKey}
}

func ShouldFallbackForStatus(statusCode int) bool {
	if statusCode >= 500 {
		return true
	}
	switch statusCode {
	case http.StatusTooManyRequests, http.StatusUnauthorized, http.StatusForbidden:
		return true
	default:
		return false
	}
}
