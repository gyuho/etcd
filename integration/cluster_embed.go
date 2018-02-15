package integration

import (
	"fmt"
	"net/url"
	"os"
	"sync/atomic"
	"testing"

	"github.com/coreos/etcd/embed"
)

func mustNewMember2(t *testing.T, mcfg memberConfig) *member {
	m := &member{}

	cfg := embed.NewConfig()
	cfg.Name = mcfg.name

	clientScheme := schemeFromTLSInfo(mcfg.clientTLS)
	peerScheme := schemeFromTLSInfo(mcfg.peerTLS)

	curl := url.URL{Scheme: clientScheme, Host: newUnixAddr()}
	cfg.ACUrls, cfg.LCUrls = []url.URL{curl}, []url.URL{curl}

	purl := url.URL{Scheme: peerScheme, Host: newUnixAddr()}
	cfg.APUrls, cfg.LPUrls = []url.URL{purl}, []url.URL{purl}

	cfg.InitialCluster = fmt.Sprintf("%s=%s", cfg.Name, cfg.APUrls[0].String())

	cfg.GRPCKeepAliveMinTime = mcfg.grpcKeepAliveMinTime
	cfg.GRPCKeepAliveInterval = mcfg.grpcKeepAliveInterval
	cfg.GRPCKeepAliveTimeout = mcfg.grpcKeepAliveTimeout

	return m
}

func newUnixAddr() string {
	return fmt.Sprintf("unix-addr:%05d%05d",
		basePort+atomic.AddInt64(&localListenCount, 1),
		os.Getpid())
}
