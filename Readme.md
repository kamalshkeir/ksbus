# KSBus

KSBus is a zero-configuration event bus written in Go, designed to facilitate real-time data sharing and synchronization between Go servers, JavaScript clients, and Python. It's particularly useful for building applications that require real-time communication, such as chat applications or live updates.

## Features

- **Zero Configuration**: Easy to set up and use without the need for complex configurations.
- **Cross-Language Support**: Supports communication between Go servers and clients, JavaScript clients, and Python clients.
- **Real-Time Data Sharing**: Enables real-time data synchronization and broadcasting.
- **Auto TLS**: Automatically generates and renews Let's Encrypt certificates for secure communication, simplifying the setup of HTTPS servers.

## Installation

To install KSBus, use the following command:

```sh
go get github.com/kamalshkeir/ksbus@v1.3.8
```

## Usage

### Server Setup

To set up a KSBus server, you can use the `NewServer` function. Here's a basic example:

```go
package main

import (
	"fmt"
	"net/http"

	"github.com/kamalshkeir/lg"
	"github.com/kamalshkeir/ksbus"
	"github.com/kamalshkeir/ksmux"
	"github.com/kamalshkeir/ksmux/ws"
)

func main() {
	bus := ksbus.NewServer(ksbus.ServerOpts{
		Address: ":9313",
		// OnDataWS: func(data map[string]any, conn *ws.Conn, originalRequest *http.Request) error {
		// 	fmt.Println("srv OnDataWS:", data)
		// 	return nil
		// },
	})
    // bus.Id = "server-9313"
    fmt.Println("connected as",bus.Id)
	
    app := bus.App // get router
	app.LocalStatics("JS", "/js") //server static dir
	lg.CheckError(app.LocalTemplates("examples/client-js")) // handle templates dir

	bus.OnDataWs(func(data map[string]any, conn *ws.Conn, originalRequest *http.Request) error {
		fmt.Println("srv OnDataWS:", data)
		return nil // if error, it will be returned to the client
	})

	bus.OnId(func(data map[string]any) { //recv on bus.Id from PublishToID
		fmt.Println("srv OnId:", data)
	})

	unsub := bus.Subscribe("server1", func(data map[string]any, unsub ksbus.Unsub) {
		fmt.Println(data)
		// unsub.Unsubscribe()
	})

    // unsub.Unsubscribe()

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
	bus.Run()
}
```



### Client Setup (JavaScript)

For JavaScript clients, you can use the provided `Bus.js` file. Here's a basic example of how to use it:

```html
<!DOCTYPE html>
<html lang="en">

<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Index</title>
</head>

<body>
    <h1>Index</h1>
    <input type="text" id="inpp">
    <button type="button" id="btn">Send</button>
    <script src="/js/Bus.js"></script>
    <script>
        let bus = new Bus({ Id: "browser" });
        bus.OnId = (d) => {
            console.log("recv OnId", d)
        }
        bus.OnDataWS = (d, wsConn) => {
            console.log("recv OnData", d)
        }
        bus.OnOpen = () => {
            console.log("connected", bus.Id)

            unsub = bus.Subscribe("index.html",(data,unsub) => { // receive once, then unsub
                console.log(data);
                unsub.Unsubscribe();
            })

            // or you can unsub from the outside using returned unsub from bus.Subscribe

            btn.addEventListener("click", (e) => {
                e.preventDefault();
                // publish and wait for recv
                bus.PublishToIDWaitRecv(inpp.value, { "cxwcwxcc": "hi from browser" }, (data) => {
                    console.log("onRecv:", data)
                }, (eventID, id) => {
                    console.log(`${id} did not receive ${eventID}`);
                })
            })
        }
    </script>
</body>

</html>
```


### Client Setup (Go)

For Go clients, you can use the `NewClient` function. Here's a basic example:

```go
package main

import (
	"fmt"

	"github.com/kamalshkeir/lg"
	"github.com/kamalshkeir/ksbus"
	"github.com/kamalshkeir/ksmux/ws"
)

func main() {
	client, err := ksbus.NewClient(ksbus.ClientConnectOptions{
		Id:      "go-client",
		Address: "localhost:9313",
        Secure: false,
		OnDataWs: func(data map[string]any, conn *ws.Conn) error {
			fmt.Println("ON OnDataWs:", data)
			return nil
		},
		OnId: func(data map[string]any, subs ksbus.Unsub) {
			fmt.Println("ON OnId:", data)
		},
	})
	if lg.CheckError(err) {
		return
	}

	fmt.Println("CLIENT connected as", client.Id)

	client.Subscribe("go-client", func(data map[string]any, sub ksbus.Unsub) {
		fmt.Println("ON sub go-client:", data)
	})

	client.Publish("server1", map[string]any{
		"data": "hello from go client",
	})

	client.PublishToIDWaitRecv("browser", map[string]any{
		"data": "hello from go client",
	}, func(data map[string]any) {
		fmt.Println("onRecv:", data)
	}, func(eventId, id string) {
		fmt.Println("not received:", eventId, id)
	})
	client.Run()
}
```


### Client Setup (Python)

For Python clients, you can install pkg ksbus

```sh
pip install ksbus==1.2.9
```


```py
from ksbus import Bus


def OnId(data):
    print("OnId:",data)

def OnOpen(bus):
    print("connected as ",bus.Id)
    bus.PublishToIDWaitRecv("browser",{
        "data":"hi from pure python"
    },lambda data:print("OnRecv:",data),lambda event_id:print("OnFail:",event_id))

if __name__ == '__main__':
    Bus({
        'Id': 'py',
        'Address': 'localhost:9313',
        'OnId': OnId,
        'OnOpen':OnOpen},
        block=True)
```


##### you can handle authentication or access to certain topics using to [Global Handlers](#global-handlers)


# API

### Internal Bus (No websockets, use channels to handle topic communications)

```go
func New() *Bus

func (b *Bus) Subscribe(topic string, fn func(data map[string]any, unsub Unsub), onData ...func(data map[string]any)) Unsub

func (b *Bus) Unsubscribe(topic string)

func (b *Bus) Publish(topic string, data map[string]any)

func (b *Bus) PublishToID(id string, data map[string]any)

func (b *Bus) PublishWaitRecv(topic string, data map[string]any, onRecv func(data map[string]any), onExpire func(eventId string, topic string)) error

func (b *Bus) PublishToIDWaitRecv(id string, data map[string]any, onRecv func(data map[string]any), onExpire func(eventId string, id string)) error

func (b *Bus) RemoveTopic(topic string)
```

### Server Bus (use the internal bus and a websocket server)
```go
func NewServer(bus ...*Bus) *Server

func (s *Server) OnServerData(fn func(data any, conn *ws.Conn)) // used when receiving data from other ksbus server (not client) using server.PublishToServer(addr string, data map[string]any, secure ...bool) error

func (s *Server) OnDataWs(fn func(data map[string]any, conn *ws.Conn, originalRequest *http.Request) error) // triggered after upgrading connection to websockets and each time on recv new message 

func (s *Server) OnId(fn func(data map[string]any)) // used when server bus receive data on his ID 'server.Id'

func (s *Server) WithPprof(path ...string) // enable std library pprof endpoints /debug/pprof/...

func (s *Server) WithMetrics(httpHandler http.Handler, path ...string) // take prometheus handler as input and serve /metrics if no path specified

func (s *Server) Subscribe(topic string, fn func(data map[string]any, unsub Unsub)) (unsub Unsub)

func (s *Server) Unsubscribe(topic string)

func (srv *Server) Publish(topic string, data map[string]any)

func (s *Server) PublishToID(id string, data map[string]any) // every connection ws (go client, python client, js client and the server), all except the internal bus, each connection should have a unique id that you can send data specificaly to it (not all subscribers on a topic, only a specific connection)

func (s *Server) PublishWaitRecv(topic string, data map[string]any, onRecv func(data map[string]any), onExpire func(eventId string, topic string))

func (s *Server) PublishToIDWaitRecv(id string, data map[string]any, onRecv func(data map[string]any), onExpire func(eventId string, toID string))

func (s *Server) RemoveTopic(topic string)

func (s *Server) PublishToServer(addr string, data map[string]any, secure ...bool) error // send to another ksbus server


func (s *Server) Run() // Run without TLS
func (s *Server) RunTLS(cert string, certKey string) // RunTLS with TLS from cert files
func (s *Server) RunAutoTLS(subDomains ...string) // RunAutoTLS generate letsencrypt certificates for server.Address and subDomains and renew them automaticaly before expire, so you only need to provide a domainName(server.Address). 
```

### Client Bus GO

```go
type ClientConnectOptions struct {
	Id           string
	Address      string
	Secure       bool
	Path         string // default ksbus.ServerPath
	Autorestart  bool
	RestartEvery time.Duration
	OnDataWs     func(data map[string]any, conn *ws.Conn) error // before data is distributed on different topics
	OnId         func(data map[string]any, unsub Unsub) // used when client bus receive data on his ID 'client.Id'
}

func NewClient(opts ClientConnectOptions) (*Client, error)

func (client *Client) Subscribe(topic string, handler func(data map[string]any, unsub Unsub)) Unsub

func (client *Client) Unsubscribe(topic string)

func (client *Client) Publish(topic string, data map[string]any)

func (client *Client) PublishToServer(addr string, data map[string]any, secure ...bool)

func (client *Client) PublishToID(id string, data map[string]any)

func (client *Client) PublishWaitRecv(topic string, data map[string]any, onRecv func(data map[string]any), onExpire func(eventId string, topic string))

func (client *Client) PublishToIDWaitRecv(id string, data map[string]any, onRecv func(data map[string]any), onExpire func(eventId string, id string))

func (client *Client) RemoveTopic(topic string)

func (client *Client) Close() error

func (client *Client) Run()

```

### Client Bus JS

```js
class Bus {
    /**
     * Bus can be initialized without any param 'let bus = new Bus()'
     * @param {object} options "default: {...}"
     * @param {string} options.Id "default: uuid"
     * @param {string} options.Address "default: window.location.host"
     * @param {string} options.Path "default: /ws/bus"
     * @param {boolean} options.Secure "default: false"
     * @param {boolean} options.AutoRestart "default: false"
     * @param {number} options.RestartEvery "default: 10"
     */

Subscribe(topic, handler)
Unsubscribe(topic)
Publish(topic, data)
PublishWaitRecv(topic, data, onRecv, onExpire)
PublishToIDWaitRecv(id, data, onRecv, onExpire)
PublishToServer(addr, data, secure)
PublishToID(id, data)
RemoveTopic(topic)
```

### Client Bus PY

```py
class Bus:
    def __init__(self, options, block=False):
        self.Address = options.get('Address', 'localhost')
        self.Path = options.get('Path', '/ws/bus')
        self.scheme = 'ws://'
        if options.get('Secure', False):
            self.scheme = 'wss://'
        self.full_address = self.scheme + self.addr + self.path
        self.conn = None
        self.topic_handlers = {}
        self.AutoRestart = options.get('AutoRestart', False)
        self.RestartEvery = options.get('restartEvery', 5)
        self.OnOpen = options.get('OnOpen', lambda bus: None)
        self.OnClose = options.get('OnClose', lambda: None)
        self.OnDataWs = options.get('OnDataWs', None)
        self.OnId = options.get('OnId', lambda data: None)
        self.Id = options.get('Id') or self.makeId(12)

def Subscribe(self, topic, handler)
def Unsubscribe(self, topic)
def Publish(self, topic, data)
def PublishToID(self, id, data)
def RemoveTopic(self, topic)
def PublishWaitRecv(self, topic, data, onRecv, onExpire)
def PublishToIDWaitRecv(self, id, data, onRecv, onExpire)
def PublishToServer(self, addr, data, secure)
```

## Global Handlers 
```go
OnUpgradeWS   = func(r *http.Request) bool { return true }
// also server.OnDataWs can be used for auth
```


## Example Python Client
```sh
pip install ksbus==1.2.8
```

With FastApi example:
```py
from typing import Union

from fastapi import FastAPI
# ksbus import
from ksbus import Bus
from pydantic import BaseSettings


class Settings(BaseSettings):
    bus: Bus = None

settings= Settings()
app = FastAPI()


def initBus():
    Bus({
        'Id': 'py',
        'Address': 'localhost:9313',
        'OnId': OnId,
        'OnOpen':onOpen})

def onOpen(bus):
    print("connected")
    settings.bus=bus
    print(settings.bus)
    bus.Publish("server",{
        "data":"hi i am python"
    })
    bus.Subscribe("client",lambda data,subs : print(data))



@app.get("/")
async def read_root():
    if settings.bus == None:
        initBus()
    return {"Hello": "World"}


@app.get("/items/{item_id}")
async def read_item(item_id: int, q: Union[str, None] = None):
    return {"item_id": item_id, "q": q}
```