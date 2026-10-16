package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"umbraco-cli/internal/config"
)

func fileCacheAt(t *testing.T) (TokenCache, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".umbraco", "live.token.json")
	return FileTokenCache(path, testConfig.BaseURL, testConfig.ClientID, "profile:live"), path
}

func TestFileCacheRoundTripsWithOwnerOnlyPermissions(t *testing.T) {
	cache, path := fileCacheAt(t)
	key := CacheKey("profile:live", testConfig)
	expires := time.Now().Add(4 * time.Minute).Truncate(time.Second)
	cache.Store(key, "access-token-1", expires)
	token, at, ok := cache.Load(key)
	if !ok || token != "access-token-1" || !at.Equal(expires) {
		t.Fatalf("round trip: %q %s %v", token, at, ok)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(path)
		dir, _ := os.Stat(filepath.Dir(path))
		if info.Mode().Perm() != 0o600 || dir.Mode().Perm() != 0o700 {
			t.Fatalf("expected file 0600 in a created 0700 directory, got %v and %v", info.Mode().Perm(), dir.Mode().Perm())
		}
	}
	raw, _ := os.ReadFile(path)
	var item map[string]any
	_ = json.Unmarshal(raw, &item)
	if item["key"] != key || item["baseUrl"] != testConfig.BaseURL || item["clientId"] != testConfig.ClientID || item["selection"] != "profile:live" {
		t.Fatalf("the file names what the token belongs to: %s", raw)
	}
	if strings.Contains(string(raw), testConfig.ClientSecret) {
		t.Fatalf("the client secret must never be written: %s", raw)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("no temporary files may be left behind: %v", entries)
	}
}

func TestAKeyMismatchIsAMiss(t *testing.T) {
	cache, _ := fileCacheAt(t)
	cache.Store(CacheKey("profile:live", testConfig), "access-token-1", time.Now().Add(time.Minute))
	other := testConfig
	other.BaseURL = "https://dev.example.test"
	if _, _, ok := cache.Load(CacheKey("profile:live", other)); ok {
		t.Fatalf("a token for another base URL must not be used")
	}
}

func TestAGroupOrWorldReadableFileIsAMissAndIsReplaced(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits do not apply on Windows")
	}
	cache, path := fileCacheAt(t)
	key := CacheKey("profile:live", testConfig)
	cache.Store(key, "access-token-1", time.Now().Add(time.Minute))
	for _, mode := range []os.FileMode{0o644, 0o640, 0o604} {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		if _, _, ok := cache.Load(key); ok {
			t.Fatalf("a %v file must not be read", mode)
		}
	}
	cache.Store(key, "access-token-2", time.Now().Add(time.Minute))
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("the next write must leave 0600, got %v", info.Mode().Perm())
	}
	if token, _, ok := cache.Load(key); !ok || token != "access-token-2" {
		t.Fatalf("after the rewrite the token reads again: %q %v", token, ok)
	}
}

func TestASymlinkedTokenFileIsAMiss(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	cache, path := fileCacheAt(t)
	key := CacheKey("profile:live", testConfig)
	target := filepath.Join(t.TempDir(), "elsewhere.json")
	other := FileTokenCache(target, testConfig.BaseURL, testConfig.ClientID, "profile:live")
	other.Store(key, "planted", time.Now().Add(time.Minute))
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := cache.Load(key); ok {
		t.Fatalf("a symlink must not be followed")
	}
}

// Parallel invocations (agents run several at once) share the file: every
// read sees a complete token or a miss, never a torn file, and the file is
// valid at the end.
func TestParallelInvocationsNeverSeeATornFile(t *testing.T) {
	cache, path := fileCacheAt(t)
	key := CacheKey("profile:live", testConfig)
	// Once the file exists, an atomic rename always leaves a complete one:
	// a miss would mean a reader saw a partial write.
	cache.Store(key, "token-seed-"+strings.Repeat("x", 2000), time.Now().Add(time.Minute))
	var wg sync.WaitGroup
	errs := make(chan string, 1000)
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				cache.Store(key, fmt.Sprintf("token-%d-%d-%s", w, i, strings.Repeat("x", 2000)), time.Now().Add(time.Minute))
				token, _, ok := cache.Load(key)
				if !ok {
					errs <- "miss"
				} else if !strings.HasPrefix(token, "token-") || !strings.HasSuffix(token, strings.Repeat("x", 2000)) {
					errs <- token[:20]
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for bad := range errs {
		t.Fatalf("a read saw a torn token: %q", bad)
	}
	if _, _, ok := cache.Load(key); !ok {
		t.Fatalf("the file must be valid after the race")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("no temporary files may be left behind: %d entries", len(entries))
	}
}

// Parallel providers over one file: all authenticate, and a racing double
// fetch is the worst case.
func TestParallelProvidersShareTheCachedToken(t *testing.T) {
	cache, _ := fileCacheAt(t)
	key := CacheKey("profile:live", testConfig)
	requests := 0
	var mu sync.Mutex
	client := tokenServer(&requests)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := NewCached(testConfig, client, cache, key).AccessToken(context.Background())
			mu.Lock()
			defer mu.Unlock()
			if err != nil || token == "" {
				t.Errorf("invocation failed: %q %v", token, err)
			}
		}()
	}
	wg.Wait()
	before := requests
	if _, err := NewCached(testConfig, client, cache, key).AccessToken(context.Background()); err != nil || requests != before {
		t.Fatalf("a later invocation must reuse the cached token: %v, %d → %d requests", err, before, requests)
	}
}

func TestTokenFilesFollowTheSelectedConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := t.TempDir()
	t.Chdir(work)
	custom := filepath.Join(work, "dev.json")
	if path, _, err := TokenCacheFile(config.LoadOptions{Profile: "live"}, testConfig); err != nil || path != filepath.Join(home, ".umbraco", "live.token.json") {
		t.Fatalf("profile: got %s (%v)", path, err)
	}
	// A --config token stays under ~/.umbraco/tokens, never next to the
	// config file, so it cannot land in a project repository.
	configPath, _, err := TokenCacheFile(config.LoadOptions{ConfigPath: custom}, testConfig)
	if err != nil || filepath.Dir(configPath) != filepath.Join(home, ".umbraco", "tokens") || !strings.HasPrefix(filepath.Base(configPath), "config-") || strings.Contains(configPath, work) {
		t.Fatalf("config: got %s (%v)", configPath, err)
	}
	other, _, _ := TokenCacheFile(config.LoadOptions{ConfigPath: filepath.Join(work, "live.json")}, testConfig)
	if other == configPath {
		t.Fatalf("two config files must not share a token file")
	}
	// Profile and --config paths do not depend on the config's contents.
	broken := config.Config{}
	if again, _, _ := TokenCacheFile(config.LoadOptions{ConfigPath: custom}, broken); again != configPath {
		t.Fatalf("a --config token path must not need the config: %s vs %s", again, configPath)
	}
	path, key, err := TokenCacheFile(config.LoadOptions{}, testConfig)
	if err != nil || path != filepath.Join(home, ".umbraco", "tokens", key+".token.json") {
		t.Fatalf("unselected: got %s (%v)", path, err)
	}
	// The active profile counts as selected.
	if err := config.SetActiveProfile("dev"); err != nil {
		t.Fatal(err)
	}
	if path, _, _ := TokenCacheFile(config.LoadOptions{}, testConfig); path != filepath.Join(home, ".umbraco", "dev.token.json") {
		t.Fatalf("active profile: got %s", path)
	}
	// Profiles and their keys stay apart.
	livePath, liveKey, _ := TokenCacheFile(config.LoadOptions{Profile: "live"}, testConfig)
	devPath, devKey, _ := TokenCacheFile(config.LoadOptions{Profile: "dev"}, testConfig)
	if livePath == devPath || liveKey == devKey {
		t.Fatalf("profiles must not share a file or key")
	}
}

func TestRemoveTokenFileReportsWhetherOneWasThere(t *testing.T) {
	cache, path := fileCacheAt(t)
	cache.Store("k", "t", time.Now().Add(time.Minute))
	if removed, err := RemoveTokenFile(path); !removed || err != nil {
		t.Fatalf("first remove: %v %v", removed, err)
	}
	if removed, err := RemoveTokenFile(path); removed || err != nil {
		t.Fatalf("second remove: %v %v", removed, err)
	}
}

func TestDeleteOnlyRemovesTheTokenItWasGiven(t *testing.T) {
	cache, path := fileCacheAt(t)
	key := CacheKey("profile:live", testConfig)
	cache.Store(key, "newer", time.Now().Add(time.Minute))
	if err := cache.Delete(key, "older"); err != nil {
		t.Fatal(err)
	}
	if token, _, ok := cache.Load(key); !ok || token != "newer" {
		t.Fatalf("a 401 for an older token must not delete a newer one: %q %v", token, ok)
	}
	if err := cache.Delete(key, "newer"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("deleting the held token removes the file: %v", err)
	}
	if err := cache.Delete(key, ""); err != nil {
		t.Fatalf("deleting a missing file is not an error: %v", err)
	}
}

func TestAFIFOAtTheTokenPathIsAMissNotAHang(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no FIFOs on Windows")
	}
	cache, path := fileCacheAt(t)
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	if err := syscallMkfifo(path); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	done := make(chan bool, 1)
	go func() {
		_, _, ok := cache.Load(CacheKey("profile:live", testConfig))
		done <- ok
	}()
	select {
	case ok := <-done:
		if ok {
			t.Fatalf("a FIFO must be a miss")
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("reading a FIFO at the token path hung")
	}
}

type foreignInfo struct{ os.FileInfo }

func (foreignInfo) Sys() any { return nil }

func TestAFileOwnedByAnotherUserIsAMiss(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("owner ids do not apply on Windows")
	}
	cache, path := fileCacheAt(t)
	cache.Store("k", "t", time.Now().Add(time.Minute))
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !ownedByCurrentUser(info) {
		t.Fatalf("a file this test wrote is owned by the current user")
	}
	if ownedByCurrentUser(foreignInfo{info}) {
		t.Fatalf("file info without the owner's id must not count as owned")
	}
	if ownedByCurrentUser(withOwner(info, os.Getuid()+1)) {
		t.Fatalf("another user's file must not count as owned")
	}
}

func TestADirectoryWritableByOthersDisablesTheCache(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits do not apply on Windows")
	}
	cache, path := fileCacheAt(t)
	key := CacheKey("profile:live", testConfig)
	cache.Store(key, "stored-before", time.Now().Add(time.Minute))
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if _, _, ok := cache.Load(key); ok {
		t.Fatalf("a token in a group- or world-writable directory must not be trusted")
	}
	cache.Store(key, "stored-after", time.Now().Add(time.Minute))
	_ = os.Chmod(dir, 0o700)
	if token, _, _ := cache.Load(key); token != "stored-before" {
		t.Fatalf("no write may happen in a loose directory, got %q", token)
	}
	if info, _ := os.Stat(dir); info.Mode().Perm() != 0o700 {
		t.Fatalf("the cache must not change the directory's permissions")
	}
}

func TestAFailedWriteLeavesNoTemporaryFile(t *testing.T) {
	cache, path := fileCacheAt(t)
	// A directory at the token path makes the final rename fail.
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	cache.Store("k", "t", time.Now().Add(time.Minute))
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".token-") {
			t.Fatalf("a failed write left %s behind", entry.Name())
		}
	}
}

func TestAReadOnlyDirectoryStillAuthenticates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits do not apply on Windows")
	}
	requests := 0
	dir := filepath.Join(t.TempDir(), "ro")
	if err := os.MkdirAll(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	cache := FileTokenCache(filepath.Join(dir, "live.token.json"), testConfig.BaseURL, testConfig.ClientID, "profile:live")
	key := CacheKey("profile:live", testConfig)
	for i := 0; i < 2; i++ {
		token, err := NewCached(testConfig, tokenServer(&requests), cache, key).AccessToken(context.Background())
		if err != nil || token == "" {
			t.Fatalf("invocation %d: %q, %v", i, token, err)
		}
	}
	if requests != 2 {
		t.Fatalf("with an unwritable directory each invocation fetches its own token, got %d requests", requests)
	}
}

func TestInvalidateDeletesTheCachedFile(t *testing.T) {
	cache, path := fileCacheAt(t)
	key := CacheKey("profile:live", testConfig)
	requests := 0
	provider := NewCached(testConfig, tokenServer(&requests), cache, key)
	if _, err := provider.AccessToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the fetched token is cached: %v", err)
	}
	provider.Invalidate()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Invalidate must delete the file: %v", err)
	}
}
