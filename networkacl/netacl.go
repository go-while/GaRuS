package networkacl

import (
	"fmt"
	"strings"
	"net/http"
	"net"
	"sync"
)

var GetNetACLFunc = GetNetACL // Default assignment

const anyhost = "0.0.0.0"

// Call this during startup or when acl changes!
// caller must lock/unlock before/after calling!
// this func returns a new map!
// replace old map with this one!
func GetNetACL(acl string) map[string]struct{} {
	addrs := make(map[string]struct{})
	for _, entry := range strings.Split(acl, ",") {
		addr := strings.TrimSpace(entry)
		if addr != "" {
			addrs[addr] = struct{}{}
		}
	}

	return addrs
} // end func GetNetACL

// caller should submit a mutex that we can lock on and safely read the map!
// or make sure,that there are no other writers!
func IsNetAllowed(host string, mux *sync.RWMutex, acl map[string]struct{}, test bool) bool {
	/*
	if test {
		fmt.Printf("TEST IsNetAllowed returns true")
		return true
	}
	*/
	if mux != nil {
		mux.RLock()
		defer mux.RUnlock()
	}

	_, openWorldwide := acl[anyhost]
	if openWorldwide {
		return true
	}
	/*
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr // fallback, may already be just host
	}
	*/

	// check if remote Addr exists in map as key
	_, allowed := acl[host]
	if allowed {
		return allowed
	}
	// addr did not exist, check if there are subnets to check against
	for ipnet, _ := range acl {
		if strings.Contains(ipnet, "/") {
			allowed, err := MatchCIDR(host, ipnet)
			if err != nil {
				fmt.Printf("ERROR netacl.IsNetAllowed: MatchCidr returned err='%v'\n", err)
				return false
			}
			if allowed {
				return allowed
			}
		}
	}
	_, allowed = acl[host]
	return allowed
} // end func IsNetAllowed

func GetHost(r *http.Request) string {
	if r == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr // fallback, may already be just host
	}
	return host
}

func MatchCIDR(remoteAddr string, matchCIDR string) (bool, error) {
	ip := net.ParseIP(remoteAddr)
	if ip == nil {
		return false, fmt.Errorf("invalid IP address: %s", remoteAddr)
	}

	_, ipNet, err := net.ParseCIDR(matchCIDR)
	if err != nil {
		return false, fmt.Errorf("invalid CIDR block: %s", matchCIDR)
	}

	return ipNet.Contains(ip), nil
} // end func MatchCIDR: fully written by AI
