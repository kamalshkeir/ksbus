package main

import (
	"fmt"
	"log"
	"time"

	"github.com/kamalshkeir/ksbus"
)

func main() {
	// Create an RPC client
	rpcClient, err := ksbus.NewRPCClient(ksbus.RPCClientOptions{
		Id:           "rpc-client-imp",
		Address:      "localhost:9314",
		Autorestart:  true,
		RestartEvery: 5 * time.Second,
		// OnDataRPC: func(data map[string]any) error {
		// 	fmt.Printf("RPC Client received general message: %v\n", data)
		// 	return nil
		// },
		OnId: func(data map[string]any, unsub ksbus.RPCSubscriber) {
			fmt.Printf("RPC Client received message on ID: %v\n", data)
		},
	})
	if err != nil {
		log.Fatal("Failed to create RPC client:", err)
	}

	// Subscribe to a topic with RPC client
	rpcClient.Subscribe("rpc-client", func(data map[string]any, unsub ksbus.RPCSubscriber) {
		fmt.Printf("RPC Client received on rpc-client topic: %v\n", data)
	})

	go func() {
		for i := 0; i < 50; i++ {
			rpcClient.PublishToID("master", map[string]any{
				"msg": "hello from rpc",
			})
			time.Sleep(time.Second)
		}
	}()

	// Publish messages using both clients
	// rpcClient.PublishToIDWaitRecv("master", map[string]any{
	// 	"message": "Hello from rpc client",
	// 	"time":    time.Now().String(),
	// }, func(data map[string]any) {
	// 	fmt.Println("success", data)
	// }, func(eventId, id string) {
	// 	fmt.Println("fail", eventId, id)
	// })

	rpcClient.Run()
}
