package integration

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"os"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/embed"
	"github.com/coreos/etcd/etcdserver/api/v3client"
	"github.com/coreos/etcd/internal/compactor"
	"github.com/golang/glog"
)

func TestAAA(t *testing.T) {
	tdir, err := ioutil.TempDir(os.TempDir(), "aaa")
	if err != nil {
		panic(err)
	}

	glog.Infof("removing %q", tdir)
	os.RemoveAll(tdir)
	glog.Infof("removed %q", tdir)

	defer func() {
		glog.Infof("removing %q", tdir)
		os.RemoveAll(tdir)
		glog.Infof("removed %q", tdir)
	}()

	embedded, err := newEtcd(context.Background(), tdir, 3)
	if err != nil {
		panic(err)
	}
	defer func() {
		glog.Infof("closing server!")
		// embedded.stop()
		embedded.ss[0].Close()
		glog.Infof("closed server!")
	}()

	time.Sleep(3 * time.Second)

	cli, err := clientv3.New(clientv3.Config{
		Endpoints: []string{
			embedded.cfgs[0].ACUrls[0].String(),
			// embedded.cfgs[1].ACUrls[0].String(),
			// embedded.cfgs[2].ACUrls[0].String(),
		},
	})
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	time.Sleep(3 * time.Second)

	_, err = cli.Put(context.Background(), "foo", "bar")
	fmt.Println(err)
}

type embeddedEtcd struct {
	dataDir string
	cfgs    []*embed.Config
	ss      []*embed.Etcd
}

var basePortA = 2379

func newEtcd(ctx context.Context, dataDir string, size int) (*embeddedEtcd, error) {
	srv := &embeddedEtcd{
		dataDir: dataDir,
		cfgs:    make([]*embed.Config, size),
		ss:      make([]*embed.Etcd, size),
	}
	for i := 0; i < size; i++ {
		cport := basePortA
		pport := basePortA + 1
		basePortA += 2

		cfg := embed.NewConfig()
		cfg.ClusterState = embed.ClusterStateFlagNew

		cfg.Name = fmt.Sprintf("etcd%d", i)
		cfg.Dir = filepath.Join(dataDir, cfg.Name)

		curl := url.URL{Scheme: "http", Host: fmt.Sprintf("localhost:%d", cport)}
		cfg.ACUrls, cfg.LCUrls = []url.URL{curl}, []url.URL{curl}

		purl := url.URL{Scheme: "http", Host: fmt.Sprintf("localhost:%d", pport)}
		cfg.APUrls, cfg.LPUrls = []url.URL{purl}, []url.URL{purl}

		cfg.InitialCluster = fmt.Sprintf("%s=%s", cfg.Name, cfg.APUrls[0].String())

		cfg.AutoCompactionMode = compactor.ModePeriodic
		cfg.AutoCompactionRetention = "1h" // every hour
		cfg.SnapCount = 1000               // single-node, keep minimum snapshot

		srv.cfgs[i] = cfg
	}

	iss := []string{}
	for i := 0; i < size; i++ {
		iss = append(iss, srv.cfgs[i].InitialCluster)
	}
	for i := 0; i < size; i++ {
		srv.cfgs[i].InitialCluster = strings.Join(iss, ",")
	}

	errc := make(chan error, size)
	for i := 0; i < size; i++ {
		go func(idx int) {
			glog.Infof("starting %q with endpoint %q", srv.cfgs[idx].Name, srv.cfgs[idx].ACUrls[0].String())
			s, err := embed.StartEtcd(srv.cfgs[idx])
			if err != nil {
				errc <- err
				return
			}
			srv.ss[idx] = s
			select {
			case <-srv.ss[idx].Server.ReadyNotify():
				err = nil
			case err = <-srv.ss[idx].Err():
			case <-srv.ss[idx].Server.StopNotify():
				err = fmt.Errorf("received from etcdserver.Server.StopNotify")
			case <-ctx.Done():
				err = ctx.Err()
			}
			if err != nil {
				errc <- err
				return
			}
			glog.Infof("started %q with endpoint %q", srv.cfgs[idx].Name, srv.cfgs[idx].ACUrls[0].String())
			errc <- nil
		}(i)
	}
	for i := 0; i < size; i++ {
		if err := <-errc; err != nil {
			return nil, err
		}
	}

	cli := v3client.New(srv.ss[0].Server)
	defer cli.Close()

	// issue linearized read to ensure leader election
	glog.Infof("sending GET to endpoint %q", srv.cfgs[0].ACUrls[0].String())
	_, err := cli.Get(ctx, "foo")
	glog.Infof("sent GET to endpoint %q (error: %v)", srv.cfgs[0].ACUrls[0].String(), err)

	return srv, err
}

func (srv *embeddedEtcd) stop() {
	defer func() {
		os.RemoveAll(srv.dataDir)
	}()
	glog.Info("stopping embedded etcd server")
	for _, s := range srv.ss {
		s.Close()
	}
	glog.Info("stopped embedded etcd server")
}
