package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"umbraco-cli/internal/config"
)

// TokenCache keeps an access token between invocations. Only the access
// token is cached: never the client secret, which stays in the config.
type TokenCache interface {
	Load(key string) (token string, expiresAt time.Time, ok bool)
	Store(key string, token string, expiresAt time.Time)
	// Delete removes the cached token, or with a non-empty token only while
	// the cache still holds that token. A missing entry is not an error.
	Delete(key string, token string) error
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
// only. Writes are atomic: a 0600 temporary file in the same directory,
// then a rename, so parallel invocations never see a partial file (a new
// directory is created 0700). Reads open the file without following a
// symlink or blocking on a FIFO, and use it only if the open handle is a
// regular file owned by the current user and readable by no one else;
// otherwise it is a miss and is replaced on the next write. A directory
// writable by group or others disables the cache (a miss, no write, one
// warning on stderr); the CLI never changes the directory's permissions.
// On Windows the permission bits and owner do not apply; the file inherits
// the folder's access control.
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

var looseDirectoryWarning sync.Once

// directoryUsable reports whether the token's directory may hold it: absent
// (it will be created 0700), or not writable by group or others.
func (c fileCache) directoryUsable() bool {
	dir := filepath.Dir(c.path)
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil {
		return false
	}
	if writableByOthers(info) {
		looseDirectoryWarning.Do(func() {
			fmt.Fprintf(os.Stderr, "umbraco: not caching the access token: %s is writable by group or others\n", dir)
		})
		return false
	}
	return true
}

// read returns the cached item if the file is safe to trust.
func (c fileCache) read() (cachedToken, bool) {
	if !c.directoryUsable() {
		return cachedToken{}, false
	}
	file, err := openTokenFile(c.path)
	if err != nil {
		return cachedToken{}, false
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !ownerOnly(info) || !ownedByCurrentUser(info) {
		return cachedToken{}, false
	}
	raw, err := io.ReadAll(io.LimitReader(file, 64*1024))
	if err != nil {
		return cachedToken{}, false
	}
	var item cachedToken
	if json.Unmarshal(raw, &item) != nil || item.Version != 1 || item.AccessToken == "" {
		return cachedToken{}, false
	}
	return item, true
}

func (c fileCache) Load(key string) (string, time.Time, bool) {
	item, ok := c.read()
	if !ok || item.Key != key {
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
	if !c.directoryUsable() {
		return
	}
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
	// os.CreateTemp creates the file 0600.
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
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

// Delete removes the token file, or with a non-empty token only while the
// file still holds it: a 401 for one invocation's token must not discard
// a newer token another invocation stored meanwhile.
func (c fileCache) Delete(_ string, token string) error {
	if token != "" {
		if item, ok := c.read(); ok && item.AccessToken != token {
			return nil
		}
	}
	err := os.Remove(c.path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// RemoveTokenFile deletes a token file, reporting whether one was there.
func RemoveTokenFile(path string) (bool, error) {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
