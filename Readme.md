### KSbus is a zero configuration Eventbus written in Golang, it offer an easy way to share/synchronise data between your Golang servers or between you servers and browsers(JS client) , or simply between your GO and JS clients or with Python. 
It use [Ksmux](https://github.com/kamalshkeir/ksmux)

### Any Go App can communicate with another Go Server or another Go Client. 

### JS client is written in Pure JS, you can serve a single file ./JS/bus.js

## Get Started

```sh
go get github.com/kamalshkeir/ksbus@v1.2.6
```

## You don't know where you can use it ?, here is a simple use case example:
#### let's take a discord like application for example, if you goal is to broadcast message in realtime to all room members notifying them that a new user joined the room, you can do it using pooling of course, but it's not very efficient, you can use also any broker but it will be hard to subscribe from the browser or html directly

## KSbus make it very easy, here is how:

###### Client side:

```html
<script src="path/to/Bus.js"></script>
<script>
// Bus can be initialized without any param 'let bus = new Bus()'
// @param {object} options "default: {...}" 
// @param {string} options.addr "default: window.location.host"
// @param {string} options.path "default: /ws/bus"
// @param {boolean} option.secure "default: false"
let bus = new Bus({
	autorestart: true,
	restartevery: 5
});
bus.OnOpen = () => {
    let sub = bus.Subscribe("room-client",(data,subs) => {
        // show notification
		...

		// publish
		bus.Publish("test-server",{
			"data": "Hello from browser",
		})
		// you can use subs to unsubscribe
		subs.Unsubscribe()
    });

	// or you can unsubscribe from outside the handler using the returned sub from Subscribe method
	sub.Unsubscribe();
}
</script>
```
###### Server side:
```go
bus := ksbus.NewServer()

// whenever you register a new user to the room
bus.Publish("room-client",map[string]any{
	....
})

bus.Run("localhost:9313")
```


##### you can handle authentication or access to certain topics using to [Before Handlers](#before-handlers)


### Internal Bus (No websockets, use channels to handle topic communications)

```go
// New return new Bus
func New() *Bus
// Subscribe let you subscribe to a topic and return a unsubscriber channel
func (b *Bus) Subscribe(topic string, fn func(data map[string]any, ch Channel)) (ch Channel)
func (b *Bus) Unsubscribe(ch Channel)
func (b *Bus) UnsubscribeId(topic, id string)
func (b *Bus) Publish(topic string, data map[string]any)
func (b *Bus) PublishToID(id string, data map[string]any)
func (b *Bus) PublishWaitRecv(topic string, data map[string]any, onRecv func(data map[string]any, ch Channel))
func (b *Bus) RemoveTopic(topic string)
```

### Server Bus (use the internal bus and a websocket server)
```go
func NewServer(bus ...*Bus) *Server
func (s *Server) OnData(fn func(data map[string]any))
func (s *Server) WithPprof(path ...string)
func (s *Server) WithMetrics(httpHandler http.Handler, path ...string)
func (s *Server) Subscribe(topic string, fn func(data map[string]any, ch Channel)) (ch Channel)
func (s *Server) Unsubscribe(ch Channel)
func (s *Server) Publish(topic string, data map[string]any, from ...string)
func (s *Server) PublishToID(id string, data map[string]any, from ...string)
func (s *Server) PublishWaitRecv(topic string, data map[string]any, onRecv func(data map[string]any, ch Channel), from ...string)
func (s *Server) RemoveTopic(topic string)
func (s *Server) PublishToServer(addr string, data map[string]any, secure ...bool)
func (s *Server) Run(addr string)
func (s *Server) RunTLS(addr string, cert string, certKey string)
func (s *Server) RunAutoTLS(domainName string, subDomains ...string)

// param and returned subscribe channel 
func (ch Channel) Unsubscribe() Channel
```


##### Example:
```go
func main() {
	bus := ksbus.NewServer()
	bus.App.LocalTemplates("tempss") // load template folder to be used with c.HTML
	bus.App.LocalStatics("assets","/assets/")
	
	bus.App.GET("/",func(c *ksmux.Context) {
		c.Html("index.html",nil)
	})

	bus.Subscribe("server",func(data map[string]any, ch ksbus.Channel) {
		log.Println("server recv:",data)
	})

	
	bus.App.GET("/pp",func(c *ksmux.Context) {
		bus.Publish("client",map[string]any{
			"msg":"hello from server",
		})
		c.Text("ok")
	})
	
	bus.Run(":9313")
}
```


### Client Bus GO

```go
type ClientConnectOptions struct {
	Address      string
	Secure       bool
	Path         string // default ksbus.ServerPath
	Autorestart  bool
	RestartEvery time.Duration
	OnData       func(data map[string]any)
}
func NewClient(opts ClientConnectOptions) (*Client, error)
func (client *Client) Subscribe(topic string, handler func(data map[string]any, sub *ClientSubscription)) *ClientSubscription
func (client *Client) Unsubscribe(topic string)
func (client *Client) Publish(topic string, data map[string]any)
func (client *Client) PublishToID(id string, data map[string]any)
func (client *Client) RemoveTopic(topic string)
func (client *Client) Close() error
func (client *Client) Run()
func (subscribtion *ClientSubscription) Unsubscribe() *ClientSubscription

// param and returned subscription
func (subscribtion *ClientSubscription) Unsubscribe() (*ClientSubscription)

```

##### Example
```go
func main() {
	client,err := ksbus.NewClient(ksbus.ClientConnectOptions{
        Address: "localhost:9313",
		Secure:  false,
    })
	if klog.CheckError(err){
		return
	}
	client.Subscribe("topic1",func(data map[string]any, subs *ksbus.ClientSubscription) {
		fmt.Println("client recv",data)
	})

    client.Subscribe("topic2",func(data map[string]any, subs *ksbus.ClientSubscription) {
		fmt.Println("client recv",data)
	})

	client.Run()
}
```


### Client JS
##### you can find the client ws wrapper in the repos JS folder above
```js
/**
 * Bus can be initialized without any param 'let bus = new Bus()'
 * @param {object} options "default: {...}"
 * @param {string} options.addr "default: window.location.host"
 * @param {string} options.path "default: /ws/bus"
 * @param {boolean} options.secure "default: false"
 */

/**
     * Subscribe subscribe to a topic
     * @param {string} topic 
     * @param {function handler(data: string,subscription: busSubscription) {}} handler 
     */
    Subscribe(topic,handler)
/**
     * Unsubscribe unsubscribe from topic
     * @param {string} topic 
     */
    Unsubscribe(topic)
/**
     * Publish publish to topic
     * @param {string} topic 
     * @param {object} data 
     */
    Publish(topic,data)
/**
     * PublishToID publish to client or server id
     * @param {string} id 
     * @param {object} data 
     */
     PublishToID(id,data)
/**
     * RemoveTopic remove a topic completely from the server bus
     * @param {string} topic 
     * @returns 
     */
    RemoveTopic(topic)
```

##### JS Client Example:

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">  
    <meta http-equiv="X-UA-Compatible" content="IE=edge">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Document</title>
</head>
<body>
    <h1>Index</h1>
    <button id="btn">Click</button>


<script src="/assets/js/Bus.js"></script>
<script>
let bus = new Bus({
	autorestart: true,
	restartevery: 5
});

bus.OnOpen = () => {
    let sub = bus.Subscribe("room-client",(data,subs) => {
        // show notification
		...

		// publish
		bus.Publish("test-server",{
			"data": "Hello from browser, i got room_client msg",
		})
		// you can use subs to unsubscribe
		//subs.Unsubscribe()
    });

	// or you can unsubscribe from outside the handler using the returned sub from Subscribe method
	sub.Unsubscribe();
}

btn.addEventListener("click",(e) => {
    e.preventDefault();
    // send to only one connection , our go client in this case, so this is a communication between client js and client go
    bus.Publish("test-server",{
        "msg":"hello go client from client js"
    })
})

</script>
</body>
</html>
```

## Before Handlers 
```go
OnDataWS      = func(data map[string]any, conn *ws.Conn, originalRequest *http.Request) bool { return true } // Used for go bus server and client when getting data from the ws connection
OnUpgradeWS   = func(r *http.Request) bool { return true } // Before upgrading the request to WS
OnServersData = func(data any, conn *ws.Conn) {} // when recv data from other servers via srvBus.PublishToServer
```


## Example Python Client
```sh
pip install ksbus==1.2.7
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
    Bus("localhost:9313", onOpen=onOpen)

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

Pure Python example

```py
from ksbus import Bus


# pythonTopicHandler handle topic 'python'
async def pythonTopicHandler(data,subs):
    print("recv on topic python:",data)
    # Unsubscribe
    #await subs.Unsubscribe()

def OnId(data):
    print("OnId:",data)

def OnOpen(bus):
    print("connected",bus)
    bus.PublishToIDWaitRecv("browser",{
        "data":"hi from pure python"
    },lambda data:print("OnRecv:",data),lambda event_id:print("OnFail:",event_id))

if __name__ == '__main__':
    Bus({
        'id': 'py',
        'addr': 'localhost:9313',
        'OnId': OnId,
        'OnOpen':OnOpen},
        block=True)
```