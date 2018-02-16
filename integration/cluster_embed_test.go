package integration

import (
	"context"
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/pkg/capnslog"
	"google.golang.org/grpc/grpclog"
)

func TestEmbedAAA(t *testing.T) {
	capnslog.SetGlobalLogLevel(capnslog.INFO)
	clientv3.SetLogger(grpclog.NewLoggerV2(ioutil.Discard, ioutil.Discard, ioutil.Discard))

	c := NewCluster2(t, 3)
	defer c.Terminate(t)

	c.Launch(t)

	time.Sleep(2 * time.Second)

	_, err := c.Members[0].serverClient.Put(context.TODO(), "foo", "bar")
	fmt.Println(err)

	time.Sleep(2 * time.Second)

	gr, err := c.Members[0].serverClient.Get(context.TODO(), "foo")
	fmt.Println(gr, err)

	time.Sleep(2 * time.Second)
}
