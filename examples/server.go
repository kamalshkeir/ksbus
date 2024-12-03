package main

import (
	"fmt"
	"time"

	"github.com/kamalshkeir/ksbus"
	"github.com/kamalshkeir/ksmux"
	"github.com/kamalshkeir/lg"
)

func main() {
	bus := ksbus.NewServer(ksbus.ServerOpts{
		Address:        ":9313",
		WithRPCAddress: ":9314",
	})

	app := bus.App

	app.LocalStatics("JS", "/js")
	lg.CheckError(app.LocalTemplates("examples/client-js"))

	// bus.OnDataWs(func(data map[string]any, conn *ws.Conn, originalRequest *http.Request) error {
	// 	fmt.Println("srv OnDataWS:", data)
	// 	return nil
	// })

	// bus.OnId(func(data map[string]any) {
	// 	fmt.Println("srv OnId:", data)
	// })

	bus.Subscribe("server1", func(data map[string]any, unsub ksbus.Unsub) {
		for i := 0; i < 50; i++ {
			time.Sleep(500 * time.Millisecond)
			bus.Publish("rpc", map[string]any{
				"msg": "got you",
			})
		}

		// unsub.Unsubscribe()
	})

	app.Get("/", func(c *ksmux.Context) {
		c.Html("index.html", nil)
	})

	app.Get("/pp", func(c *ksmux.Context) {
		bus.PublishToIDWaitRecv("go-client", map[string]any{
			"msg": "hello from server",
		},
			func(data map[string]any) {
				fmt.Println("go-client received my message, response:", data)
			}, func(eventId, toID string) {
				fmt.Println(toID, "didn't recv message with topic", eventId)
			})
		c.Text("ok")
	})

	fmt.Println("server connected AS", bus.ID)
	bus.Run()
}
