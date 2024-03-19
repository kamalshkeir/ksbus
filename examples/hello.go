package main

import (
	"fmt"

	"github.com/kamalshkeir/klog"
	"github.com/kamalshkeir/ksbus"
	"github.com/kamalshkeir/ksmux"
)

func main() {
	bus := ksbus.NewServer()

	app := bus.App

	app.LocalStatics("JS", "/js")
	klog.CheckError(app.LocalTemplates("temps"))

	bus.OnData(func(data map[string]any) {
		fmt.Println("srv OnData:", data)
	})
	bus.Subscribe("server1", func(data map[string]any, ch ksbus.Channel) {
		fmt.Println("server1 recv:", data)
		if v, ok := data["my_id"]; ok {
			bus.PublishToIDWaitRecv(v.(string), map[string]any{
				"msg": "got you",
				"id":  bus.ID,
			}, func(data map[string]any, ch ksbus.Channel) {
				fmt.Println("client got the message", data)
			}, nil)
		}
	})
	app.Get("/", func(c *ksmux.Context) {
		c.Html("index.html", nil)
	})

	bus.Run(":9313")

}
