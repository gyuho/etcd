package main

/*
 */

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/coreos/etcd/clientv3"
)

func main() {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:            []string{"localhost:2379"},
		DialTimeout:          5 * time.Second,
		DialKeepAliveTime:    2 * time.Second,
		DialKeepAliveTimeout: 2 * time.Second,
	})
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	go func() {
		for {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			_, err := cli.Put(ctx, "___test", "")
			cancel()
			fmt.Println("put", err)
			time.Sleep(time.Second * 2)
		}
	}()
	go func() {
		ch := cli.Watch(context.Background(), "___test")
		for ev := range ch {
			if ev.Err() != nil {
				fmt.Println("watch error:", err)
				continue
			}
			fmt.Println("watch received:", ev.Events)
		}
	}()

	time.Sleep(5 * time.Second)
	println()
	fmt.Printf("netstat with clients without packet drop\n%s\n", getNetStat())
	println()

	time.Sleep(5 * time.Second)
	println()
	fmt.Println("injecting packet drop 1")
	injectPacketDrop()
	fmt.Println("injecting packet drop 2")
	println()

	time.Sleep(5 * time.Second)
	println()
	fmt.Printf("netstat after packet drop\n%s\n", getNetStat())
	println()

	time.Sleep(5 * time.Second)
	println()
	fmt.Println("recovering packet drop 1")
	recoverPacketDrop()
	fmt.Println("recovering packet drop 2")
	println()

	time.Sleep(10 * time.Second)
	println()
	fmt.Printf("netstat with clients without packet drop\n%s\n", getNetStat())
	println()

	notifier := make(chan os.Signal, 1)
	signal.Notify(notifier, syscall.SIGINT, syscall.SIGTERM)
	fmt.Println("exiting with", <-notifier)
}

func getNetStat() string {
	const header = `Proto Recv-Q Send-Q Local Address           Foreign Address         State       User       Inode      PID/Program name`
	cmd := exec.Command("bash", "-c", "netstat -nape | grep ':2379'")
	out, err := cmd.Output()
	if err != nil {
		panic(err)
	}
	return header + "\n" + string(out)
}

func injectPacketDrop() {
	cmd := exec.Command("bash", "-c", "iptables -A INPUT -j DROP -p tcp --destination-port 2379; iptables -A INPUT -j DROP -p tcp --source-port 2379")
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}

func recoverPacketDrop() {
	cmd := exec.Command("bash", "-c", "iptables -D INPUT -j DROP -p tcp --destination-port 2379; iptables -D INPUT -j DROP -p tcp --source-port 2379")
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}

/*
https://github.com/coreos/etcd/issues/9212


# run with sudo
go build -v .
sudo ./aaa


# reset packet drop
sudo iptables -D INPUT -j DROP -p tcp --destination-port 2379
sudo iptables -D INPUT -j DROP -p tcp --source-port 2379

# add packet drop
sudo iptables -A INPUT -j DROP -p tcp --destination-port 2379
sudo iptables -A INPUT -j DROP -p tcp --source-port 2379

# restore packet drop
sudo iptables -D INPUT -j DROP -p tcp --destination-port 2379
sudo iptables -D INPUT -j DROP -p tcp --source-port 2379

# watch netstate
sudo netstat - | grep ':2379'
*/
