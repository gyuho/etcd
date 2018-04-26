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

// Package endpoint resolves etcd entpoints using grpc targets of the form 'endpoint://<clientId>/<endpoint>'.
package endpoint

import (
	"fmt"
	"net/url"
	"strings"
	"sync"

	"google.golang.org/grpc/resolver"
)

const (
	scheme = "endpoint"
)

var (
	targetPrefix = fmt.Sprintf("%s://", scheme)

	bldr *builder
)

func init() {
	bldr = &builder{
		clientResolvers: make(map[string]*Resolver),
	}
	resolver.Register(bldr)
}

type builder struct {
	clientResolvers map[string]*Resolver
	sync.RWMutex
}

// Build creates or reuses an etcd resolver for the etcd cluster name identified by the authority part of the target.
func (b *builder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOption) (resolver.Resolver, error) {
	if len(target.Authority) < 1 {
		return nil, fmt.Errorf("'etcd' target scheme requires non-empty authority identifying etcd cluster being routed to")
	}
	r := b.getResolver(target.Authority)
	r.cc = cc
	if r.addrs != nil {
		r.NewAddress(r.addrs)
	}
	return r, nil
}

func (b *builder) getResolver(clientId string) *Resolver {
	b.RLock()
	r, ok := b.clientResolvers[clientId]
	b.RUnlock()
	if !ok {
		r = &Resolver{
			clientId: clientId,
		}
		b.Lock()
		b.clientResolvers[clientId] = r
		b.Unlock()
	}
	return r
}

func (b *builder) addResolver(r *Resolver) {
	bldr.Lock()
	bldr.clientResolvers[r.clientId] = r
	bldr.Unlock()
}

func (b *builder) removeResolver(r *Resolver) {
	bldr.Lock()
	delete(bldr.clientResolvers, r.clientId)
	bldr.Unlock()
}

func (r *builder) Scheme() string {
	return scheme
}

// EndpointResolver gets the resolver for  given etcd cluster name.
func EndpointResolver(clientId string) *Resolver {
	return bldr.getResolver(clientId)
}

// Resolver provides a resolver for a single etcd cluster, identified by name.
type Resolver struct {
	clientId string
	cc       resolver.ClientConn
	addrs    []resolver.Address
	sync.RWMutex
}

// InitialAddrs sets the initial endpoint addresses for the resolver.
func (r *Resolver) InitialAddrs(addrs []resolver.Address) {
	r.Lock()
	r.addrs = addrs
	r.Unlock()
}

// InitialEndpoints sets the initial endpoints to for the resolver.
// This should be called before dialing. The endpoints may be updated after the dial using NewAddress.
// At least one endpoint is required.
func (r *Resolver) InitialEndpoints(eps []string) error {
	if len(eps) < 1 {
		return fmt.Errorf("At least one endpoint is required, but got: %v", eps)
	}
	r.InitialAddrs(epsToAddrs(eps...))
	return nil
}

// TODO: use balancer.epsToAddrs
func epsToAddrs(eps ...string) (addrs []resolver.Address) {
	addrs = make([]resolver.Address, 0, len(eps))
	for _, ep := range eps {
		addrs = append(addrs, resolver.Address{Addr: ep})
	}
	return addrs
}

// NewAddress updates the addresses of the resolver.
func (r *Resolver) NewAddress(addrs []resolver.Address) {
	r.Lock()
	r.addrs = addrs
	r.Unlock()
	if r.cc != nil {
		r.cc.NewAddress(addrs)
	}
}

func (*Resolver) ResolveNow(o resolver.ResolveNowOption) {}

func (r *Resolver) Close() {
	bldr.removeResolver(r)
}

// Target constructs a endpoint target with current resolver's clientId.
func (r *Resolver) Target(endpoint string) string {
	return Target(r.clientId, endpoint)
}

// Target constructs a endpoint resolver target.
func Target(clientId, endpoint string) string {
	return fmt.Sprintf("%s://%s/%s", scheme, clientId, endpoint)
}

// IsTarget checks if a given target string in an endpoint resolver target.
func IsTarget(target string) bool {
	return strings.HasPrefix(target, "endpoint://")
}

// Parse endpoint parses a endpoint of the form (http|https)://<host>*|(unix|unixs)://<path>) and returns a
// protocol ('tcp' or 'unix'), host (or filepath if a unix socket) and scheme (http, https, unix, unixs).
func ParseEndpoint(endpoint string) (proto string, host string, scheme string) {
	proto = "tcp"
	host = endpoint
	url, uerr := url.Parse(endpoint)
	if uerr != nil || !strings.Contains(endpoint, "://") {
		return proto, host, scheme
	}
	scheme = url.Scheme

	// strip scheme:// prefix since grpc dials by host
	host = url.Host
	switch url.Scheme {
	case "http", "https":
	case "unix", "unixs":
		proto = "unix"
		host = url.Host + url.Path
	default:
		proto, host = "", ""
	}
	return proto, host, scheme
}

// ParseTarget parses a endpoint://<clientId>/<endpoint> string and returns the parsed clientId and endpoint.
// If the target is malformed, an error is returned.
func ParseTarget(target string) (string, string, error) {
	noPrefix := strings.TrimPrefix(target, targetPrefix)
	if noPrefix == target {
		return "", "", fmt.Errorf("malformed target, %s prefix is required: %s", targetPrefix, target)
	}
	parts := strings.SplitN(noPrefix, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("malformed target, expected %s://<clientId>/<endpoint>, but got %s", scheme, target)
	}
	return parts[0], parts[1], nil
}
