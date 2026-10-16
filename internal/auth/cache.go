package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"umbraco-cli/internal/config"
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

// TokenCacheDisabled reports whether NoTokenCacheEnv turns the cache off.
func TokenCacheDisabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(NoTokenCacheEnv))) {
	case "", "0", "false":
		return false
	}
	return true
}

// CacheKey identifies the token of one selection (profile or config file)
// against one server and API client. A cached file whose key differs is a
// miss.
func CacheKey(selection string, cfg config.Config) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{"v1", selection, strings.TrimRight(cfg.BaseURL, "/"), cfg.ClientID}, "\n")))
	return hex.EncodeToString(sum[:])
}

// TokenCacheFile is the token file for opts and cfg and the key its content
// must carry. See config.TokenCacheSelection for the scheme; unselected
// configurations use ~/.umbraco/tokens/<key>.token.json.
func TokenCacheFile(opts config.LoadOptions, cfg config.Config) (string, string, error) {
	selection, path, err := tokenCacheSelection(opts)
	if err != nil {
		return "", "", err
	}
	key := CacheKey(selection, cfg)
	if path == "" {
		dir, err := config.TokenCacheDir()
		if err != nil {
			return "", "", err
		}
		path = filepath.Join(dir, key+".token.json")
	}
	return path, key, nil
}

// SystemTokenCache is the file token cache for opts and cfg, or nil when
// NoTokenCacheEnv turns it off or the file cannot be placed.
func SystemTokenCache(opts config.LoadOptions, cfg config.Config) (TokenCache, string) {
	if TokenCacheDisabled() {
		return nil, ""
	}
	path, key, err := TokenCacheFile(opts, cfg)
	if err != nil {
		return nil, ""
	}
	selection, _, _ := tokenCacheSelection(opts)
	return FileTokenCache(path, cfg.BaseURL, cfg.ClientID, selection), key
}

var tokenCacheSelection = config.TokenCacheSelection

// FileTokenCache keeps one token in the file at path, readable by its owner
// only (0600, in a 0700 directory it creates). Writes are atomic: a 0600
// temporary file in the same directory, then a rename, so parallel
// invocations never see a partial file. A file readable by group or others,
// or not a regular file, is a miss and is replaced on the next write. On
// Windows the permission bits do not apply; the file inherits the folder's
// access control.
//
// The file also names the base URL, client id and selection the token
// belongs to, for whoever reads it; the key decides a hit.
func FileTokenCache(path string, baseURL string, clientID string, selection string) TokenCache {
	return fileCache{path: path, baseURL: baseURL, clientID: clientID, selection: selection}
}

type fileCache struct {
	path      string
	baseURL   string
	clientID  string
	selection string
}

type cachedToken struct {
	Version     int    `json:"version"`
	Key         string `json:"key"`
	BaseURL     string `json:"baseUrl,omitempty"`
	ClientID    string `json:"clientId,omitempty"`
	Selection   string `json:"selection,omitempty"`
	AccessToken string `json:"accessToken"`
	ExpiresAt   string `json:"expiresAt"`
}

// ownerOnly reports whether info allows only its owner to read it.
func ownerOnly(info os.FileInfo) bool {
	return runtime.GOOS == "windows" || info.Mode().Perm()&0o077 == 0
}

func (c fileCache) Load(key string) (string, time.Time, bool) {
	info, err := os.Lstat(c.path)
	if err != nil || !info.Mode().IsRegular() || !ownerOnly(info) {
		return "", time.Time{}, false
	}
	raw, err := os.ReadFile(c.path)
	if err != nil {
		return "", time.Time{}, false
	}
	var item cachedToken
	if json.Unmarshal(raw, &item) != nil || item.Version != 1 || item.Key != key || item.AccessToken == "" {
		return "", time.Time{}, false
	}
	expiresAt, err := time.Parse(time.RFC3339, item.ExpiresAt)
	if err != nil {
		return "", time.Time{}, false
	}
	return item.AccessToken, expiresAt, true
}

// Store writes the token for key. A failure leaves the invocation working
// with its in-memory token.
func (c fileCache) Store(key string, token string, expiresAt time.Time) {
	payload, err := json.Marshal(cachedToken{Version: 1, Key: key, BaseURL: c.baseURL, ClientID: c.clientID, Selection: c.selection, AccessToken: token, ExpiresAt: expiresAt.UTC().Format(time.RFC3339)})
	if err != nil {
		return
	}
	dir := filepath.Dir(c.path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if os.MkdirAll(dir, 0o700) != nil {
			return
		}
	}
	tmp, err := os.CreateTemp(dir, ".token-*.tmp")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil && runtime.GOOS != "windows" {
		_ = tmp.Close()
		return
	}
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return
	}
	if tmp.Close() != nil {
		return
	}
	_ = os.Rename(tmpName, c.path)
}

// Delete removes the token file; a missing file is fine.
func (c fileCache) Delete(string) {
	_ = os.Remove(c.path)
}

// RemoveTokenFile deletes a token file, reporting whether one was there.
func RemoveTokenFile(path string) (bool, error) {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
