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
		Address:      "localhost:9314",
		Autorestart:  true,
		RestartEvery: 5 * time.Second,
		OnDataRPC: func(data map[string]any) error {
			fmt.Printf("RPC Client received general message: %v\n", data)
			return nil
		},
		OnId: func(data map[string]any, unsub ksbus.RPCSubscriber) {
			fmt.Printf("RPC Client received message on ID: %v\n", data)
		},
	})
	if err != nil {
		log.Fatal("Failed to create RPC client:", err)
	}

	// Subscribe to a topic with RPC client
	subs := rpcClient.Subscribe("rpc", func(data map[string]any, unsub ksbus.RPCSubscriber) {
		fmt.Printf("RPC Client received on test-topic: %v\n", data)
	})

	// Publish messages using both clients
	rpcClient.Publish("server1", map[string]any{
		"message": "Hello meeeeeeeeeeeeeeeeeeeeee",
		"time":    time.Now().String(),
	})

	time.Sleep(5 * time.Second)
	subs.Unsubscribe()
	fmt.Println("unsubscribed------------------")
	rpcClient.Run()
}
