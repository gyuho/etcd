// Copyright 2018 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package netutil

import (
	"net"
	"net/url"
	"sort"
	"strings"
	"sync"
)

// defaultLoopbackIPs contains all loopback interface addresses.
// (https://github.com/apple/cups/blob/818bbe7af2c235dc975249ae1231eedd8353de64/scheduler/client.c#L3559-L3562)
//
// CVE-2018-5702:
// - https://bugs.chromium.org/p/project-zero/issues/detail?id=1447#c2
// - https://github.com/transmission/transmission/pull/468
var (
	lmu                sync.RWMutex
	defaultLoopbackIPs = map[string]struct{}{
		"localhost":  {}, // localhost is loopback network interface
		"localhost.": {}, // localhost is loopback network interface
		"127.0.0.1":  {}, // IPv4
		"[::1]":      {}, // IPv6
		"::1":        {}, // IPv6
	}
)

func init() {
	ifaces, err := net.Interfaces()
	if err != nil {
		return
	}
	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			return
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip.IsLoopback() {
				lmu.Lock()
				defaultLoopbackIPs[ip.String()] = struct{}{}
				lmu.Unlock()
			}
		}
	}
}

// GetHost removes all schemes and ports from address
// and only returns the host in the address.
func GetHost(addr string) (host string) {
	addr = strings.TrimSpace(addr)
	u, _ := url.Parse(addr)
	if u != nil && u.Host != "" {
		addr = u.Host
	}

	var err error
	host, _, err = net.SplitHostPort(addr)
	if err != nil {
		// no scheme missing port (e.g. localhost)
		// or empty host with too many colons (e.g. localhost:::)
		host = addr
	}
	return host
}

// IsLoopback returns true if the connection address is over
// loopback interface (e.g. "localhost", "127.0.0.1", etc.).
func IsLoopback(addr string) (ok bool) {
	if addr == "" {
		return false
	}

	addr = GetHost(addr)

	lmu.RLock()
	_, ok = defaultLoopbackIPs[addr]
	lmu.RUnlock()
	if ok {
		return ok
	}

	return net.ParseIP(addr).IsLoopback()
}

// GetDefaultLoopbackAddrs returns all default loopback addresses.
func GetDefaultLoopbackAddrs() (addrs []string) {
	lmu.RLock()
	addrs = make([]string, 0, len(defaultLoopbackIPs))
	for addr := range defaultLoopbackIPs {
		addrs = append(addrs, addr)
	}
	lmu.RUnlock()
	sort.Strings(addrs)
	return addrs
}
