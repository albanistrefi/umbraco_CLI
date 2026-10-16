package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"time"

	"umbraco-cli/internal/config"
	"umbraco-cli/internal/keychain"
)

// TokenCache keeps an access token between invocations. Only the access
// token is cached: never the client secret, which stays in the config.
type TokenCache interface {
	Load(key string) (token string, expiresAt time.Time, ok bool)
	Store(key string, token string, expiresAt time.Time)
	Delete(key string)
}

// NoTokenCacheEnv turns the cross-invocation token cache off when set to
// anything but "", "0" or "false".
const NoTokenCacheEnv = "UMBRACO_NO_TOKEN_CACHE"

// systemStore is the credential store SystemTokenCache uses; tests swap it.
var systemStore = keychain.System

// keychainService names the keychain items the cache writes.
const keychainService = "umbraco-cli access token"

// SystemTokenCache is the token cache of the running system: the macOS
// keychain, or nil (no cache) when it is unavailable or NoTokenCacheEnv
// turns it off. There is no file fallback.
func SystemTokenCache() TokenCache {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(NoTokenCacheEnv))) {
	case "", "0", "false":
	default:
		return nil
	}
	store := systemStore()
	if _, ok := store.(keychain.Unavailable); ok {
		return nil
	}
	return KeychainTokenCache(store)
}

// KeychainTokenCache keeps tokens in store, one item per key.
func KeychainTokenCache(store keychain.Store) TokenCache {
	return keychainCache{store: store}
}

type keychainCache struct {
	store keychain.Store
}

type cachedToken struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expiresAt"`
}

func (c keychainCache) Load(key string) (string, time.Time, bool) {
	raw, err := c.store.Get(keychainService, key)
	if err != nil {
		return "", time.Time{}, false
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", time.Time{}, false
	}
	var item cachedToken
	if json.Unmarshal(decoded, &item) != nil || item.Token == "" {
		return "", time.Time{}, false
	}
	return item.Token, time.Unix(item.ExpiresAt, 0), true
}

// Store writes the token with its expiry. The value is base64 so it passes
// the keychain's character check whatever the token looks like. A failure
// leaves the invocation working with its in-memory token.
func (c keychainCache) Store(key string, token string, expiresAt time.Time) {
	payload, err := json.Marshal(cachedToken{Token: token, ExpiresAt: expiresAt.Unix()})
	if err != nil {
		return
	}
	_ = c.store.Set(keychainService, key, keychainService, base64.StdEncoding.EncodeToString(payload))
}

func (c keychainCache) Delete(key string) {
	_ = c.store.Delete(keychainService, key)
}

// CacheKey identifies the token of one profile against one server and API
// client: the profile (or config file) the credentials came from, the base
// URL and the client id, hashed so the keychain item names no host or
// client. Changing any of them misses the cache.
func CacheKey(selection string, cfg config.Config) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{"v1", selection, strings.TrimRight(cfg.BaseURL, "/"), cfg.ClientID}, "\n")))
	return hex.EncodeToString(sum[:])
}

// SelectionName names where a config came from for CacheKey: the profile,
// the explicit config file, or "" for the default resolution.
func SelectionName(opts config.LoadOptions) string {
	if path := strings.TrimSpace(opts.ConfigPath); path != "" {
		return "config:" + path
	}
	if profile := strings.TrimSpace(opts.Profile); profile != "" {
		return "profile:" + strings.ToLower(profile)
	}
	return ""
}
