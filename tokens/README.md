# tokens

**GaRuS tokens module**  
_Thread-safe token storage and management for GaRuS (GitHub-actions-Runner-upload-Server)_

---

## Overview

This Go module provides a thread-safe, in-memory authentication token store with automatic periodic reload and expiry for the [GaRuS](https://github.com/go-while/GaRuS) project.  
It is designed to manage per-repository tokens, supporting access control by network (IP/subnet) and token expiry.

**Key features:**
- Efficient, thread-safe storage and management of tokens
- Supports multiple tokens per repository
- Network ACL (IP, subnet) based access control for each token
- Token expiry and automatic cleanup
- Automatic reload from file (only when changed)
- Simple, human-readable token file format

---

## Token File Format

The tokens module expects a "passwd" file with the following format (one line per token):

```
repoName|TOKEN_VALUE|EXPIRES_AT|NETWORKS
```

- `repoName`: Name of the repository the token is valid for
- `TOKEN_VALUE`: The authentication token (minimum length: 20)
- `EXPIRES_AT`: Expiry time as a unix epoch timestamp
- `NETWORKS`: Comma-separated list of IPs or subnets allowed to use the token

Example:
```
demo-repo|XXXTOKENXXX|1867944282|127.0.0.1,[::1],192.168.12.0/24
```

- The special value `0.0.0.0` in the network list allows worldwide access (IPv4 & IPv6), overriding all other restrictions.

---

## Usage

### Import

```go
import "github.com/go-while/GaRuS/tokens"
```

### Creating a Token Store

```go
store := tokens.NewTokenStore("/path/to/passwdfile")
```
- This starts a background goroutine to auto-reload tokens when the file changes.

### Authenticating Requests

```go
result := store.Auth(repoName, tokenValue, httpRequest, false)
if result.Valid {
    // Proceed
} else {
    // result.Reason contains "expired", "repo not found", etc.
}
```

### Adding and Revoking Tokens

```go
// Add or update a token (usually handled via the passwd file)
store.AddToken("repo", "token", expiresUnix, "192.168.1.0/24")
// Remove a specific token
store.RevokeToken("repo", "token")
```

---

## API

### Types

- `type TokenStore`  
  Main thread-safe token store object.

- `type AuthResult struct { Valid bool; Reason string; Expires int64 }`  
  Result of an authentication check.

### Important Methods

- `NewTokenStore(filename string) *TokenStore`  
  Creates a new token store and starts background reloading.

- `(ts *TokenStore) Auth(repo, token string, r *http.Request, test bool) AuthResult`  
  Check if a token is valid for the given repo and request.

- `(ts *TokenStore) AddToken(repo, token string, expires int64, network string)`  
  Manually add/update a token (in-memory).

- `(ts *TokenStore) RevokeToken(repo, token string) bool`  
  Remove a token from the store.

---

## Notes

- Token file reloads are SHA256-hash based (only reloads if changed).
- Expired tokens are automatically removed from memory.
- All methods are safe for concurrent use.
- Uses [github.com/go-while/GaRuS/networkacl](https://github.com/go-while/GaRuS/networkacl) for network access checks.

---

## Example

```go
store := tokens.NewTokenStore("passwd")
result := store.Auth("demo-repo", "XXXTOKENXXX", req, false)
if result.Valid {
    fmt.Println("Authenticated!")
} else {
    fmt.Printf("Auth failed: %s\n", result.Reason)
}
```

---

## License

[MIT](https://github.com/go-while/GaRuS/blob/main/LICENSE)
