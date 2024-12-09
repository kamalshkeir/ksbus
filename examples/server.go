package main

import (
	"fmt"

	"github.com/kamalshkeir/ksbus"
	"github.com/kamalshkeir/ksmux"
	"github.com/kamalshkeir/lg"
)

func main() {
	bus := ksbus.NewServer(ksbus.ServerOpts{
		ID:             "master",
		Address:        ":9313",
		WithRPCAddress: ":9314",
		OnId: func(data map[string]any) {
			fmt.Println("OnId:", data)
		},
	})

	app := bus.App

	app.LocalStatics("JS", "/js")
	lg.CheckError(app.LocalTemplates("examples/client-js"))

	bus.Subscribe("server1", func(data map[string]any, unsub ksbus.Unsub) {
		fmt.Println("got on topic server1", data)
	})

	app.Get("/", func(c *ksmux.Context) {
		c.Html("index.html", nil)
	})

	app.Get("/pp", func(c *ksmux.Context) {
		bus.PublishWaitRecv("rpc-client", map[string]any{
			"msg": "hello from master",
		}, func(data map[string]any) {
			fmt.Println("success", data)
		}, func(eventId, toID string) {
			fmt.Println("fail", eventId, toID)
		})
		c.Text("ok")
	})

	fmt.Println("server connected AS", bus.ID)
	bus.Run()
}
