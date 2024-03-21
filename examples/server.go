package main

import (
	"fmt"
	"net/http"

	"github.com/kamalshkeir/klog"
	"github.com/kamalshkeir/ksbus"
	"github.com/kamalshkeir/ksmux"
	"github.com/kamalshkeir/ksmux/ws"
)

func main() {
	bus := ksbus.NewServer()

	app := bus.App

	app.LocalStatics("../JS", "/js")
	klog.CheckError(app.LocalTemplates("../temps"))

	bus.OnDataWs(func(data map[string]any, conn *ws.Conn, originalRequest *http.Request) error {
		fmt.Println("srv OnDataWS:", data)
		return nil
	})

	bus.OnId(func(data map[string]any) {
		fmt.Println("srv OnId:", data)
	})

	bus.Subscribe("server1", func(data map[string]any, ch ksbus.Channel) {
		fmt.Println("server1 recv:", data)
	})

	app.Get("/", func(c *ksmux.Context) {
		c.Html("index.html", nil)
	})

	app.Get("/pp", func(c *ksmux.Context) {
		bus.Bus.Publish("server1", map[string]any{
			"data": "INTERNAL hello",
		})
		c.Text("ok")
	})

	fmt.Println("server1 connected as", bus.ID)
	bus.Run(":9313")

}
