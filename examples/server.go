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

	app.LocalStatics("JS", "/js")
	klog.CheckError(app.LocalTemplates("examples/client-js"))

	bus.OnDataWs(func(data map[string]any, conn *ws.Conn, originalRequest *http.Request) error {
		fmt.Println("srv OnDataWS:", data)
		return nil
	})

	bus.OnId(func(data map[string]any) {
		fmt.Println("srv OnId:", data)
	})

	bus.Subscribe("server1", func(data map[string]any, unsub ksbus.Unsub) {
		fmt.Println(data)
		// unsub.Unsubscribe()
	})

	app.Get("/", func(c *ksmux.Context) {
		c.Html("index.html", nil)
	})

	app.Get("/pp", func(c *ksmux.Context) {
		bus.Bus.Publish("server1", map[string]any{
			"data": "hello from INTERNAL",
		})
		c.Text("ok")
	})

	fmt.Println("server1 connected as", bus.ID)
	bus.Run(":9313")

}
