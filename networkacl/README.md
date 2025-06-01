# networkacl

**GaRuS network ACL module**
_Network-based access control utility functions for IP and subnet whitelisting/blacklisting_

---

## Overview

This Go module provides network-based access control logic for the [GaRuS](https://github.com/go-while/GaRuS) project and other authentication systems.
It enables efficient, thread-safe checking of whether a remote host (IP address) is allowed access based on a configurable list of allowed IP addresses and CIDR subnets.

**Key features:**
- Fast lookup for exact IP matches and CIDR (subnet) matches
- Supports worldwide open access with `"0.0.0.0"`
- Designed for thread-safe use (with caller-provided mutex)
- Simple integration into HTTP middleware and authentication layers

---

## Usage

### Import

```go
import "github.com/go-while/GaRuS/networkacl"
```

### Creating an ACL Map

Call `GetNetACL` with a comma-separated list of IPs and subnets to create a map:

```go
acl := networkacl.GetNetACL("127.0.0.1,192.168.0.0/24,0.0.0.0")
```
- Each entry is trimmed for whitespace.
- The special value `"0.0.0.0"` enables worldwide (open) access.

### Checking Access

Use `IsNetAllowed` to check if a host is allowed:

```go
var mu sync.RWMutex
allowed := networkacl.IsNetAllowed("192.168.0.5", &mu, acl, false)
if allowed {
    // Access granted
} else {
    // Access denied
}
```
- Provide a pointer to a mutex that locks the ACL map, or nil if single-threaded.
- The `test` flag can be used for forced allow (not used by default).

### Integration with HTTP Requests

Get the remote host IP from an HTTP request:

```go
host := networkacl.GetHost(r) // r is *http.Request
allowed := networkacl.IsNetAllowed(host, &mu, acl, false)
```

---

## API

- `func GetNetACL(acl string) map[string]struct{}`
  Parses a comma-separated ACL string into a map of allowed IPs and subnets.

- `func IsNetAllowed(host string, mux *sync.RWMutex, acl map[string]struct{}, test bool) bool`
  Checks if a host is allowed, supporting mutex locking and subnet matches.

- `func GetHost(r *http.Request) string`
  Extracts the host IP from an HTTP request.

- `func MatchCIDR(remoteAddr, matchCIDR string) (bool, error)`
  Returns true if the IP matches the given CIDR subnet.

---

## Example

```go
import (
    "net/http"
    "sync"
    "github.com/go-while/GaRuS/networkacl"
)

var mu sync.RWMutex
var acl = networkacl.GetNetACL("127.0.0.1,10.0.0.0/8,0.0.0.0")

func handler(w http.ResponseWriter, r *http.Request) {
    host := networkacl.GetHost(r)
    if networkacl.IsNetAllowed(host, &mu, acl, false) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("Access granted"))
    } else {
        w.WriteHeader(http.StatusForbidden)
        w.Write([]byte("Access denied"))
    }
}
```

---

## Notes

- Callers must manage locking when updating or reading the ACL map in concurrent scenarios.
- `"0.0.0.0"` in the ACL map allows all hosts (IPv4 & IPv6).
- Subnet/CIDR matches are supported (e.g. `"192.168.1.0/24"`).
- Designed for easy integration with token-based auth modules.

---

## License

[MIT](https://github.com/go-while/GaRuS/blob/main/LICENSE)