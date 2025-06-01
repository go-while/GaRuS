package tokens

import (
	"github.com/go-while/GaRuS/networkacl"
	"os"
	"testing"
	"time"
	"fmt"
)

// mockNetworkACL disables network checks for token validation in tests
func init() {
	// Override networkacl.GetNetACL for tests
    networkacl.GetNetACLFunc = func(network string) map[string]struct{} {
        return map[string]struct{}{}
    }
}

func writePasswdFile(t *testing.T, lines []string) string {
	tmpfile, err := os.CreateTemp("", "tokens_test_passwd")
	if err != nil {
		t.Fatalf("Failed to create temp passwd file: %v", err)
	}
	for _, line := range lines {
		if _, err := tmpfile.WriteString(line + "\n"); err != nil {
			t.Fatalf("Failed to write to temp passwd file: %v", err)
		}
	}
	tmpfile.Close()
	return tmpfile.Name()
}

func TestTokenStore_LoadAndAuth(t *testing.T) {
	now := time.Now().Unix()
	future := now + 3600
	past := now - 3600
	validToken := "toktoktoktoktoktoktoktoktok"
	expiredToken := "expiredtoktoktoktoktoktoktok"
	shortToken := "short"
	repo := "myrepo"
	otherRepo := "otherrepo"
	lines := []string{
		fmt.Sprintf("%s|%s|%d|0.0.0.0", repo, validToken, future),                // valid1
		fmt.Sprintf("%s|%s|%d|127.0.0.1", repo, validToken, future),              // valid2
		fmt.Sprintf("%s|%s|%d|[::1]", repo, validToken, future),                  // valid3
		fmt.Sprintf("%s|%s|%d|127.0.0.1,[::1]", repo, validToken, future),        // valid4
		fmt.Sprintf("%s|%s|%d|0.0.0.0", repo, expiredToken, past),                // expired
		fmt.Sprintf("%s|%s|%d|0.0.0.0", repo, shortToken, past),                  // short token
		fmt.Sprintf("%s|%s|%d|0.0.0.0", otherRepo, validToken, future),           // wrong repo
		fmt.Sprintf("%s|%s|%d|0.0.0.0", otherRepo, expiredToken, future),         // token not found
		"bad|format|line",                                                        // bad format, ignored
	}

	passwd := writePasswdFile(t, lines)
	defer os.Remove(passwd)

	ts := &TokenStore{
		secure: make(map[string]Tokens),
		passwd: passwd,
	}

	if err := ts.LoadTokens(); err != nil {
		t.Fatalf("LoadTokens error: %v", err)
	}

	// Auth: valid1
	res := ts.Auth(repo, validToken, nil, true)
	if !res.Valid {
		t.Errorf("Expected valid token, got %+v", res)
	}
	if res.Reason != "valid" {
		t.Errorf("Expected reason 'valid', got '%s'", res.Reason)
	}

	// Auth: valid2
	res = ts.Auth(repo, validToken, nil, true)
	if !res.Valid {
		t.Errorf("Expected valid token, got %+v", res)
	}
	if res.Reason != "valid" {
		t.Errorf("Expected reason 'valid', got '%s'", res.Reason)
	}

	// Auth: valid3
	res = ts.Auth(repo, validToken, nil, true)
	if !res.Valid {
		t.Errorf("Expected valid token, got %+v", res)
	}
	if res.Reason != "valid" {
		t.Errorf("Expected reason 'valid', got '%s'", res.Reason)
	}

	// Auth: valid4
	res = ts.Auth(repo, validToken, nil, true)
	if !res.Valid {
		t.Errorf("Expected valid token, got %+v", res)
	}
	if res.Reason != "valid" {
		t.Errorf("Expected reason 'valid', got '%s'", res.Reason)
	}

	// Auth: expired
	res = ts.Auth(repo, expiredToken, nil, true)
	if res.Valid || res.Reason != "token not found" {
		t.Errorf("Expected expired, got %+v", res)
	}

	// Auth: short token
	res = ts.Auth(repo, shortToken, nil, true)
	if res.Valid || (res.Reason != "token len" && res.Reason != "repo not found") {
		t.Errorf("Expected expired, got %+v", res)
	}

	// Auth: wrong repo
	res = ts.Auth("notarepo", validToken, nil, true)
	if res.Valid || res.Reason != "repo not found" {
		t.Errorf("Expected repo not found, got %+v", res)
	}

	// Auth: token not found
	res = ts.Auth(repo, "notatokennotatokennotatokennotatoken", nil, true)
	if res.Valid || res.Reason != "token not found" {
		t.Errorf("Expected token not found, got %+v", res)
	}

	// Auth: same token different repo
	res = ts.Auth(otherRepo, validToken, nil, true)
	if !res.Valid {
		t.Errorf("Expected valid token for otherRepo, got %+v", res)
	}
}

func TestTokenStore_RevokeToken(t *testing.T) {
	now := time.Now().Unix()
	future := now + 3600
	token := "toktoktoktoktoktoktoktoktok"
	repo := "myrepo"
	lines := []string{
		fmt.Sprintf("%s|%s|%d|0.0.0.0", repo, token, future),
		fmt.Sprintf("%s|%s|%d|0.0.0.0", repo, token+"2", future),
	}
	passwd := writePasswdFile(t, lines)
	defer os.Remove(passwd)

	ts := &TokenStore{
		secure: make(map[string]Tokens),
		passwd: passwd,
	}
	if err := ts.LoadTokens(); err != nil {
		t.Fatalf("LoadTokens error: %v", err)
	}

	// Should be valid before revoke
	res := ts.Auth(repo, token, nil, true)
	if !res.Valid {
		t.Fatalf("Token should be valid before revoke")
	}

	// Should be valid before revoke
	res = ts.Auth(repo, token+"2", nil, true)
	if !res.Valid {
		t.Fatalf("Token should be valid before revoke")
	}

	// Revoke
	ok := ts.RevokeToken(repo, token)
	if !ok {
		t.Fatalf("RevokeToken should return true")
	}

	// Should not be valid after revoke
	res = ts.Auth(repo, token, nil, true)
	if res.Valid || res.Reason != "token not found" {
		t.Errorf("Expected token not found after revoke, got %+v", res)
	}

	// Revoke again: should return false
	ok = ts.RevokeToken(repo, token)
	if ok {
		t.Errorf("Expected revoke to return false for already revoked token")
	}

	// Revoke token2
	ok = ts.RevokeToken(repo, token+"2")
	if !ok {
		t.Fatalf("RevokeToken should return true")
	}

	// Revoke token2 again: should return false
	ok = ts.RevokeToken(repo, token+"2")
	if ok {
		t.Errorf("Expected revoke to return false for already revoked token")
	}
}

func TestTokenStore_ExpiredTokenNotLoaded(t *testing.T) {
	now := time.Now().Unix()
	past := now - 3600
	future := now + 3600
	token := "toktoktoktoktoktoktoktoktokoktoktoktoktok"
	repo := "myrepoExired"
	lines := []string{
		fmt.Sprintf("%s|%s|%d|0.0.0.0", repo, token, past),
		fmt.Sprintf("%s|%s|%d|0.0.0.0", repo, token+"2", future),
		fmt.Sprintf("%s|%s|%d|0.0.0.0", repo+"2", token+"3", future),
	}
	passwd := writePasswdFile(t, lines)
	defer os.Remove(passwd)

	ts := &TokenStore{
		secure: make(map[string]Tokens),
		passwd: passwd,
	}
	if err := ts.LoadTokens(); err != nil {
		t.Fatalf("LoadTokens error: %v", err)
	}

	// Should not be valid
	res := ts.Auth(repo, token, nil, true)
	if res.Valid || res.Reason != "token not found" {
		t.Errorf("Expected token not found for expired token, got %+v", res)
	}
}
