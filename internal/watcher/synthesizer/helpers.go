package synthesizer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/watcher/diff"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

// StableIDGenerator generates stable, deterministic IDs for auth entries.
// It uses SHA256 hashing with collision handling via counters.
// It is not safe for concurrent use.
type StableIDGenerator struct {
	counters map[string]int
}

// NewStableIDGenerator creates a new StableIDGenerator instance.
func NewStableIDGenerator() *StableIDGenerator {
	return &StableIDGenerator{counters: make(map[string]int)}
}

// Next generates a stable ID based on the kind and parts.
// Returns the full ID (kind:hash) and the short hash portion.
func (g *StableIDGenerator) Next(kind string, parts ...string) (string, string) {
	if g == nil {
		return kind + ":000000000000", "000000000000"
	}
	hasher := sha256.New()
	hasher.Write([]byte(kind))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		hasher.Write([]byte{0})
		hasher.Write([]byte(trimmed))
	}
	digest := hex.EncodeToString(hasher.Sum(nil))
	if len(digest) < 12 {
		digest = fmt.Sprintf("%012s", digest)
	}
	short := digest[:12]
	key := kind + ":" + short
	index := g.counters[key]
	g.counters[key] = index + 1
	if index > 0 {
		short = fmt.Sprintf("%s-%d", short, index)
	}
	return fmt.Sprintf("%s:%s", kind, short), short
}

// ApplyAuthExcludedModelsMeta applies excluded models metadata to an auth entry.
// It computes a hash of excluded models and sets the auth_kind attribute.
// For OAuth entries, perKey (from the JSON file's excluded-models field) is merged
// with the global oauth-excluded-models config for the provider.
func ApplyAuthExcludedModelsMeta(auth *coreauth.Auth, cfg *config.Config, perKey []string, authKind string) {
	if auth == nil || cfg == nil {
		return
	}
	authKindKey := strings.ToLower(strings.TrimSpace(authKind))
	seen := make(map[string]struct{})
	add := func(list []string) {
		for _, entry := range list {
			if trimmed := strings.TrimSpace(entry); trimmed != "" {
				key := strings.ToLower(trimmed)
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
			}
		}
	}
	if authKindKey == "apikey" {
		add(perKey)
	} else {
		// For OAuth: merge per-account excluded models with global provider-level exclusions
		add(perKey)
		if cfg.OAuthExcludedModels != nil {
			providerKey := strings.ToLower(strings.TrimSpace(auth.Provider))
			add(cfg.OAuthExcludedModels[providerKey])
		}
	}
	combined := make([]string, 0, len(seen))
	for k := range seen {
		combined = append(combined, k)
	}
	sort.Strings(combined)
	hash := diff.ComputeExcludedModelsHash(combined)
	if auth.Attributes == nil {
		auth.Attributes = make(map[string]string)
	}
	if hash != "" {
		auth.Attributes["excluded_models_hash"] = hash
	}
	// Store the combined excluded models list so that routing can read it at runtime
	if len(combined) > 0 {
		auth.Attributes["excluded_models"] = strings.Join(combined, ",")
	}
	if authKind != "" {
		auth.Attributes["auth_kind"] = authKind
	}
}

// ApplyAPIKeyBindings enforces per-API-key credential restrictions based on
// the top-level api-key-bindings configuration.
//
// Semantics:
//   - A credential referenced by one or more bindings gets "allowed_api_keys"
//     set to the union of those API keys (whitelist — only listed keys may use it).
//   - A credential NOT referenced by any binding gets "denied_api_keys" set to
//     all bound API keys (blacklist — bound keys are restricted to their own
//     credentials and must not fall through to unbound ones).
//   - API keys that do not appear in any binding are unrestricted.
//
// Both attributes are stored as sorted, deduplicated CSV strings.
func ApplyAPIKeyBindings(auths []*coreauth.Auth, bindings []config.APIKeyBinding) {
	if len(bindings) == 0 || len(auths) == 0 {
		return
	}

	// Collect all bound API keys (deduplicated) and build reverse map.
	boundKeySet := make(map[string]struct{})
	credToKeys := make(map[string]map[string]struct{})
	for _, b := range bindings {
		ak := strings.TrimSpace(b.APIKey)
		if ak == "" {
			continue
		}
		boundKeySet[ak] = struct{}{}
		for _, cred := range b.AllowedCredentials {
			cred = strings.TrimSpace(cred)
			if cred == "" {
				continue
			}
			if credToKeys[cred] == nil {
				credToKeys[cred] = make(map[string]struct{})
			}
			credToKeys[cred][ak] = struct{}{}
		}
	}
	if len(boundKeySet) == 0 {
		// All api-keys were blank — treat as no bindings and clean up stale attrs.
		for _, a := range auths {
			if a != nil && a.Attributes != nil {
				delete(a.Attributes, "allowed_api_keys")
				delete(a.Attributes, "denied_api_keys")
			}
		}
		return
	}

	// Pre-compute sorted denied list (all bound keys) for unbound credentials.
	deniedList := sortedKeys(boundKeySet)
	deniedCSV := strings.Join(deniedList, ",")

	for _, a := range auths {
		if a == nil {
			continue
		}
		if a.Attributes == nil {
			a.Attributes = make(map[string]string)
		}
		// Merge exact match and parent match for Gemini virtual auths (union).
		// e.g. if "file.json" is bound to keyA and "file.json::project" to keyB,
		// the virtual auth gets both keyA and keyB.
		keys := mergeKeySets(credToKeys[a.ID], nil)
		if parent := a.Attributes["gemini_virtual_parent"]; parent != "" {
			keys = mergeKeySets(keys, credToKeys[parent])
		}
		if len(keys) > 0 {
			// Credential is bound → whitelist.
			a.Attributes["allowed_api_keys"] = strings.Join(sortedKeys(keys), ",")
			delete(a.Attributes, "denied_api_keys")
		} else {
			// Credential is unbound → deny all bound keys.
			a.Attributes["denied_api_keys"] = deniedCSV
			delete(a.Attributes, "allowed_api_keys")
		}
	}
}

// sortedKeys returns the keys of a set as a sorted slice.
func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// mergeKeySets returns the union of two key sets. Returns nil if both are nil/empty.
func mergeKeySets(a, b map[string]struct{}) map[string]struct{} {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	merged := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		merged[k] = struct{}{}
	}
	for k := range b {
		merged[k] = struct{}{}
	}
	return merged
}

// addConfigHeadersToAttrs adds header configuration to auth attributes.
// Headers are prefixed with "header:" in the attributes map.
func addConfigHeadersToAttrs(headers map[string]string, attrs map[string]string) {
	if len(headers) == 0 || attrs == nil {
		return
	}
	for hk, hv := range headers {
		key := strings.TrimSpace(hk)
		val := strings.TrimSpace(hv)
		if key == "" || val == "" {
			continue
		}
		attrs["header:"+key] = val
	}
}
