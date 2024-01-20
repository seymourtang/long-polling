// gossip.go
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/google/uuid"
	"github.com/hashicorp/memberlist"
)

type Event struct {
}

func (e Event) NotifyJoin(node *memberlist.Node) {
	log.Printf("new node join:%s", node.String())
}

func (e Event) NotifyLeave(node *memberlist.Node) {
	log.Printf("new node leave:%s", node.String())
}

func (e Event) NotifyUpdate(node *memberlist.Node) {
	log.Printf("new node update:%s", node.String())
}

type broadcast struct {
	msg    []byte
	notify chan<- struct{}
}

func (b broadcast) Invalidates(bb memberlist.Broadcast) bool {
	// TODO implement me
	panic("implement me")
}

func (b broadcast) Message() []byte {
	// TODO implement me
	panic("implement me")
}

func (b broadcast) Finished() {
	// TODO implement me
	panic("implement me")
}

func main() {
	local := flag.Int("port", 7946, "the port of current node")
	node := flag.String("node", "0.0.0.0", "the ip address of another node")
	flag.Parse()

	// 使用 Create 方法创建一个成员实例, 这个成员实例就代表着一群的机器
	c := memberlist.DefaultLocalConfig()
	hostname, _ := os.Hostname()
	c.Name = hostname + uuid.New().String()
	c.BindPort = *local
	c.Events = Event{}
	list, err := memberlist.Create(c)
	if err != nil {
		panic(err)
	}
	// br := &memberlist.TransmitLimitedQueue{
	// 	NumNodes:       list.NumMembers,
	// 	RetransmitMult: 3,
	// }
	// 使用 Join 方法加入一个已经存在的机器群
	// 这里的参数是一个 string 切片，只要填写这个集群中的一台机器即可，
	// gossip 协议会将这个“流言”散播给机器群的所有机器

	_, err = list.Join([]string{*node})
	if err != nil {
		panic(err)
	}

	for _, node := range list.Members() {
		fmt.Println(node.Name, node.Addr)
	}
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	_ = list.Shutdown()
}
