package main

import (
	"fmt"

	"github.com/kamalshkeir/klog"
	"github.com/kamalshkeir/ksbus"
	"github.com/kamalshkeir/ksmux/ws"
)

func main() {
	client, err := ksbus.NewClient(ksbus.ClientConnectOptions{
		Id:      "go-client",
		Address: "localhost:9313",
		OnDataWs: func(data map[string]any, conn *ws.Conn) error {
			fmt.Println("ON OnDataWs:", data)
			return nil
		},
		OnId: func(data map[string]any, subs *ksbus.ClientSubscription) {
			fmt.Println("ON OnId:", data)
		},
	})
	if klog.CheckError(err) {
		return
	}

	fmt.Println("CLIENT connected as", client.Id)

	client.Subscribe("go-client", func(data map[string]any, sub *ksbus.ClientSubscription) {
		fmt.Println("ON sub go-client:", data)
	})

	client.Publish("server1", map[string]any{
		"data": "hello from go client",
	})

	client.PublishToIDWaitRecv("browser", map[string]any{
		"data": "hello from go client",
	}, func(data map[string]any, sub *ksbus.ClientSubscription) {
		fmt.Println("onRecv:", data)
		sub.Unsubscribe()
	}, func(eventId, id string) {
		fmt.Println("not received:", eventId, id)
	})
	client.Run()
}
