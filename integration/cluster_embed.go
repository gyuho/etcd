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

package integration

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coreos/etcd/client"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/embed"
	"github.com/coreos/etcd/etcdserver/api/v3client"
)

type member2 struct {
	*embed.Config
	srv *embed.Etcd

	// serverClient is a clientv3 that directly calls the etcdserver.
	serverClient *clientv3.Client

	keepDataDirTerminate     bool
	clientMaxCallSendMsgSize int
	clientMaxCallRecvMsgSize int
}

func mustNewMember2(t *testing.T, mcfg memberConfig) *member2 {
	cfg := embed.NewConfig()
	cfg.ClusterState = embed.ClusterStateFlagNew
	cfg.Name = mcfg.name

	var err error
	cfg.Dir, err = ioutil.TempDir(os.TempDir(), "etcd")
	if err != nil {
		t.Fatal(err)
	}

	clientScheme := schemeFromTLSInfo(mcfg.clientTLS)
	peerScheme := schemeFromTLSInfo(mcfg.peerTLS)

	curl := url.URL{Scheme: clientScheme, Host: newUnixAddr()}
	cfg.ACUrls, cfg.LCUrls = []url.URL{curl}, []url.URL{curl}
	if mcfg.clientTLS != nil {
		cfg.ClientTLSInfo = *mcfg.clientTLS
	}

	purl := url.URL{Scheme: peerScheme, Host: newUnixAddr()}
	cfg.APUrls, cfg.LPUrls = []url.URL{purl}, []url.URL{purl}
	if mcfg.peerTLS != nil {
		cfg.PeerTLSInfo = *mcfg.peerTLS
	}

	cfg.TickMs = 10
	cfg.ElectionMs = 100

	cfg.InitialCluster = fmt.Sprintf("%s=%s", cfg.Name, cfg.APUrls[0].String())

	if mcfg.quotaBackendBytes > 0 {
		cfg.QuotaBackendBytes = mcfg.quotaBackendBytes
	}
	if mcfg.maxTxnOps > 0 {
		cfg.MaxTxnOps = mcfg.maxTxnOps
	}
	if mcfg.maxRequestBytes > 0 {
		cfg.MaxRequestBytes = mcfg.maxRequestBytes
	}
	if mcfg.grpcKeepAliveMinTime > 0 {
		cfg.GRPCKeepAliveMinTime = mcfg.grpcKeepAliveMinTime
	}
	if mcfg.grpcKeepAliveInterval > 0 {
		cfg.GRPCKeepAliveInterval = mcfg.grpcKeepAliveInterval
	}
	if mcfg.grpcKeepAliveTimeout > 0 {
		cfg.GRPCKeepAliveTimeout = mcfg.grpcKeepAliveTimeout
	}

	return &member2{
		Config: cfg,
		clientMaxCallRecvMsgSize: mcfg.clientMaxCallRecvMsgSize,
		clientMaxCallSendMsgSize: mcfg.clientMaxCallSendMsgSize,
	}
}

func (m *member2) Launch() error {
	plog.Printf("launching %q on %q", m.Name, m.APUrls[0].String())

	var err error
	m.srv, err = embed.StartEtcd(m.Config)
	if err != nil {
		return err
	}

	select {
	case <-m.srv.Server.ReadyNotify():
		m.serverClient = v3client.New(m.srv.Server)
		plog.Printf("launched %q(%s) on %q", m.Name, m.srv.Server.ID().String(), m.APUrls[0].String())
	case err = <-m.srv.Err():
	case <-m.srv.Server.StopNotify():
		err = fmt.Errorf("received from etcdserver.Server.StopNotify")
	case <-time.After(5 * time.Second):
		err = errors.New("took too long to start embed.Etcd")
	}
	return err
}

func (c *cluster2) mustNewMember(t *testing.T) *member2 {
	return mustNewMember2(t,
		memberConfig{
			name:                     fmt.Sprint(rand.Int()),
			peerTLS:                  c.cfg.PeerTLS,
			clientTLS:                c.cfg.ClientTLS,
			quotaBackendBytes:        c.cfg.QuotaBackendBytes,
			maxTxnOps:                c.cfg.MaxTxnOps,
			maxRequestBytes:          c.cfg.MaxRequestBytes,
			grpcKeepAliveMinTime:     c.cfg.GRPCKeepAliveMinTime,
			grpcKeepAliveInterval:    c.cfg.GRPCKeepAliveInterval,
			grpcKeepAliveTimeout:     c.cfg.GRPCKeepAliveTimeout,
			clientMaxCallSendMsgSize: c.cfg.ClientMaxCallSendMsgSize,
			clientMaxCallRecvMsgSize: c.cfg.ClientMaxCallRecvMsgSize,
		})
}

type cluster2 struct {
	cfg     *ClusterConfig
	Members []*member2
}

func newCluster2(t *testing.T, cfg *ClusterConfig) *cluster2 {
	c := &cluster2{cfg: cfg}
	ms := make([]*member2, cfg.Size)
	for i := 0; i < cfg.Size; i++ {
		ms[i] = c.mustNewMember(t)
	}
	c.Members = ms
	c.fillClusterForMembers()
	return c
}

func (c *cluster2) fillClusterForMembers() {
	if c.cfg.DiscoveryURL != "" {
		// cluster will be discovered
		return
	}

	addrs := make([]string, len(c.Members))
	for i, m := range c.Members {
		addrs[i] = m.InitialClusterFromName(m.Name)
	}
	clusterStr := strings.Join(addrs, ",")
	for _, m := range c.Members {
		m.InitialCluster = clusterStr
	}
	return
}

func NewCluster2(t *testing.T, size int) *cluster2 {
	return newCluster2(t, &ClusterConfig{Size: size})
}

func newUnixAddr() string {
	return fmt.Sprintf("addr:%05d%05d",
		basePort+atomic.AddInt64(&localListenCount, 1),
		os.Getpid())
}

func (c *cluster2) HTTPMembers() []client.Member {
	ms := []client.Member{}
	for _, m := range c.Members {
		cm := client.Member{Name: m.Name}
		for _, u := range m.APUrls {
			cm.PeerURLs = append(cm.PeerURLs, u.String())
		}
		for _, u := range m.ACUrls {
			cm.ClientURLs = append(cm.ClientURLs, u.String())
		}
		ms = append(ms, cm)
	}
	return ms
}

func (c *cluster2) waitMembersMatch(t *testing.T, membs []client.Member) {
	for _, u := range c.URLs() {
		cc := MustNewHTTPClient(t, []string{u}, c.cfg.ClientTLS)
		ma := client.NewMembersAPI(cc)
		for {
			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
			ms, err := ma.List(ctx)
			cancel()
			if err == nil && isMembersEqual(ms, membs) {
				break
			}
			time.Sleep(tickDuration)
		}
	}
}

func (c *cluster2) Launch(t *testing.T) {
	errc := make(chan error)
	for _, m := range c.Members {
		// Members are launched in separate goroutines because if they boot
		// using discovery url, they have to wait for others to register to continue.
		go func(m *member2) {
			errc <- m.Launch()
		}(m)
	}
	for range c.Members {
		if err := <-errc; err != nil {
			t.Fatalf("error setting up member: %v", err)
		}
	}
	// wait cluster to be stable to receive future client requests
	c.waitMembersMatch(t, c.HTTPMembers())
	c.waitVersion()
}

func (c *cluster2) URL(i int) string {
	return c.Members[i].LCUrls[0].String()
}

// URLs returns a list of all active client URLs in the cluster
func (c *cluster2) URLs() []string {
	return _getMembersURLs2(c.Members)
}

func _getMembersURLs2(members []*member2) []string {
	urls := make([]string, 0)
	for _, m := range members {
		select {
		case <-m.srv.Server.StopNotify():
			continue
		default:
		}
		for _, u := range m.LCUrls {
			urls = append(urls, u.String())
		}
	}
	return urls
}

func (c *cluster2) waitVersion() {
	for _, m := range c.Members {
		for {
			if m.srv.Server.ClusterVersion() != nil {
				break
			}
			time.Sleep(tickDuration)
		}
	}
}

// Close stops the member's etcdserver and closes its connections
func (m *member2) Close() {
	plog.Infof("stopping %q(%s) on %q", m.Name, m.srv.Server.ID().String(), m.APUrls[0].String())
	m.srv.Server.HardStop()
	m.srv.Close()

	var cerr error
	select {
	case cerr = <-m.srv.Err():
	case <-m.srv.Server.StopNotify():
		cerr = fmt.Errorf("received from EtcdServer.StopNotify")
	}
	if cerr != nil {
		plog.Warningf("shutdown with %q", cerr.Error())
	} else {
		plog.Infof("shutdown with no error")
	}
	plog.Infof("stopped %q(%s) on %q", m.Name, m.srv.Server.ID().String(), m.APUrls[0].String())
}

func (m *member2) Terminate(t *testing.T) {
	plog.Printf("terminating %q(%s) on %q", m.Name, m.srv.Server.ID().String(), m.APUrls[0].String())
	m.Close()
	if !m.keepDataDirTerminate {
		if err := os.RemoveAll(m.Config.Dir); err != nil {
			t.Fatal(err)
		}
	}
	plog.Printf("terminated %q(%s) on %q", m.Name, m.srv.Server.ID().String(), m.APUrls[0].String())
}

func (c *cluster2) Terminate(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(len(c.Members))
	for _, m := range c.Members {
		go func(mm *member2) {
			defer wg.Done()
			mm.serverClient.Close()
			mm.Terminate(t)
		}(m)
	}
	wg.Wait()
}
