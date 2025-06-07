package tokens

/*
 * GaRuS - Github-actions-Runner-upload-Server - importable tokens module
 *
 * Thread-safe token storage
 *
 */

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-while/GaRuS/networkacl"
)

// passwd file format:
// repo|token|expires (unix epoch timestamp)| ip or subnet
// repoName|XXXTOKENXXX|1867944282|127.0.0.1,[::1],192.168.12.0/24

// if there is a "0.0.0.0" anywhere in the network string for a token
// note: a repo can have many tokens!
// "0.0.0.0" will allow access from worldwide on ip4 & ip6
// regardless of any other networks set for this token!

const (
	MinTokenLen      = 20
	MaxTokenLen      = 1024
	SecondsPerMinute = 60
	SecondsPerHour   = 3600
	SecondsPerDay    = 86400
)

// Token represents a single authentication token.
type Token struct {
	mux     sync.RWMutex
	netacl  map[string]struct{}
	expires int64
}

// Tokens is a map of token strings to Token pointers.
type Tokens map[string]*Token // map[repo][authToken]Token{}

// TokenStore provides thread-safe token management.
type TokenStore struct {
	mux    sync.RWMutex
	secure map[string]Tokens
	passwd string
	hash   string
}

// AuthResult represents the outcome of an Auth check.
type AuthResult struct {
	Valid   bool
	Reason  string // "valid", "expired", "repo not found", etc.
	Expires int64
}

// NewTokenStore creates a new TokenStore and starts background reload.
func NewTokenStore(filename string) *TokenStore {
	ts := &TokenStore{
		secure: make(map[string]Tokens),
		passwd: filename,
	}
	go ts.WatchTokenFile()
	return ts
} // end func NewTokenStore

// LoadTokens reloads the tokens from the passwd file if changed.
func (ts *TokenStore) LoadTokens() error {
	// Compute current SHA256 hash of passwd file
	hash, err := fileSHA256(ts.passwd)
	if err != nil {
		return err
	}
	if ts.hash != "" && hash == ts.hash {
		// No change, skip loading
		fmt.Printf("LoadTokens: no change file='%s'\n", ts.passwd)
		return nil
	}
	ts.hash = hash

	file, err := os.Open(ts.passwd)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	i := 0
	ts.mux.Lock()
	defer ts.mux.Unlock()
	if ts.secure == nil {
		ts.secure = make(map[string]Tokens)
	}
	//newTokens := make(map[string]Tokens)
	for scanner.Scan() {
		i++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		data := strings.Split(line, "|")
		if len(data) != 4 {
			fmt.Printf("ERROR reading line %d from %s\n", i, ts.passwd)
			continue
		}
		if len(data[1]) < MinTokenLen {
			fmt.Printf("ERROR loading token from file='%s': line %d\n", ts.passwd, i)
			continue
		}
		if strings.TrimSpace(data[0]) == "" ||
			strings.TrimSpace(data[1]) == "" ||
			strings.TrimSpace(data[2]) == "" ||
			strings.TrimSpace(data[3]) == "" {
			fmt.Printf("ERROR reading fields from file='%s': line %d\n", ts.passwd, i)
			continue
		}
		expires, err := parseUnixTimestamp(data[2])
		if err != nil {
			fmt.Printf("ERROR parsing timestamp from file='%s': line %d err='%v'\n", ts.passwd, i, err)
			continue
		}
		repo := strings.TrimSpace(data[0])
		token := strings.TrimSpace(data[1])
		if expires < time.Now().Unix() {
			// read expired token from file: delete from memory!
			go ts.RevokeToken(repo, token)
			continue
		}
		network := strings.TrimSpace(data[3])
		go ts.AddToken(repo, token, expires, network)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
} // end func ts.LoadTokens

// Auth checks a token's validity and returns a detailed result.
func (ts *TokenStore) Auth(repo, token string, r *http.Request, test bool) AuthResult {
	if len(token) < MinTokenLen {
		return AuthResult{Valid: false, Reason: "token len"}
	}
	ts.mux.RLock()
	defer ts.mux.RUnlock()
	repoTokens, ok := ts.secure[repo]
	if !ok {
		return AuthResult{Valid: false, Reason: "repo not found"}
	}
	if t, exists := repoTokens[token]; exists {
		if !networkacl.IsNetAllowed(networkacl.GetHost(r), &t.mux, t.netacl, test) { //t.mux = repoTokens[token].mux
			return AuthResult{Valid: false, Reason: "netacl", Expires: t.expires}
		}

		if t.expires < time.Now().Unix() {
			return AuthResult{Valid: false, Reason: "expired", Expires: t.expires}
		}
		// all good!
		return AuthResult{Valid: true, Reason: "valid", Expires: t.expires}
	}
	return AuthResult{Valid: false, Reason: "token not found"}
} // end func Auth

// AddToken adds (or updates a token) in the in-memory store for a given repo.
func (ts *TokenStore) AddToken(repo string, token string, expires int64, network string) {
	if expires < time.Now().Unix() {
		fmt.Printf("WARN: tried to add an expired token='%s...' repo='%s'\n", token[:3], repo)
		return
	}
	ts.mux.Lock()
	defer ts.mux.Unlock()

	if ts.secure[repo] == nil {
		ts.secure[repo] = make(Tokens)
	}
	repoTokens, ok := ts.secure[repo]
	if !ok {
		// repo not found
		fmt.Printf("ERROR in AddToken: created repo but was not found?!\n")
		return
	}
	if _, exists := repoTokens[token]; exists {
		// update expiry value if needed
		repoTokens[token].mux.Lock()
		if expires != repoTokens[token].expires {
			repoTokens[token].expires = expires
		}
		repoTokens[token].netacl = networkacl.GetNetACLFunc(network)
		repoTokens[token].mux.Unlock()
	} else {
		// token does not exist: create new entry
		ts.secure[repo][token] = &Token{
			expires: expires,
			netacl:  networkacl.GetNetACLFunc(network),
		}
		fmt.Printf("Loaded Token: %s:xxx:%d netacl='%v' rem=(%d sec [%s])\n", repo, expires, ts.secure[repo][token].netacl, expires-time.Now().Unix(), FormatDurationHuman(expires-time.Now().Unix()))
	}
	return
} // end func ts.AddToken

// RevokeToken removes a token from the in-memory store for a given repo.
func (ts *TokenStore) RevokeToken(repo, token string) bool {
	ts.mux.Lock()
	defer ts.mux.Unlock()
	repoTokens, ok := ts.secure[repo]
	if !ok {
		return false
	}
	if _, exists := repoTokens[token]; exists {
		repoTokens[token].mux.Lock()
		delete(repoTokens, token)
		repoTokens[token].mux.Unlock()
		// Optional: if no tokens remain for repo, remove repo entry
		if len(repoTokens) == 0 {
			delete(ts.secure, repo)
		}
		return true
	}
	return false
} // end func ts.RevokeToken

// Goroutine to reload tokens every minute and delete expired ones
func (ts *TokenStore) WatchTokenFile() {
	for {
		if err := ts.LoadTokens(); err != nil {
			fmt.Printf("ERROR loading tokens err='%v'\n", err)
		}
		time.Sleep(time.Minute)
		ts.mux.Lock()
		for repo, repoTokens := range ts.secure {
			for key, token := range repoTokens {
				if token.expires < time.Now().Unix() {
					delete(repoTokens, key)
					fmt.Printf("TokenStore: Deleted expired token='%s...' from repo='%s'\n", key[3:], repo)
				}
			}
		}
		ts.mux.Unlock()
	}
} // end func ts.WatchTokenFile
