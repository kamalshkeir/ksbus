### KSbus is a zero configuration Eventbus written in Golang, it offer an easy way to share/synchronise data between your Golang servers or between you servers and browsers(JS client) , or simply between your GO and JS clients or with Python. 
It use [Kmux](https://github.com/kamalshkeir/kmux)
# What's New:
- [Python client](#example-python-client) `pip install ksbus`
- JoinCombinedServer, allow you to join a combined server, first you create a server, then you join the combined [See More](#server-bus-use-the-internal-bus-and-a-websocket-server)
- SendToServer, allow to send data from serverBus to serverBus [See More](#server-bus-use-the-internal-bus-and-a-websocket-server)

### Any Go App can communicate with another Go Server or another Go Client. 

### JS client is written in Pure JS, you can server it using kmux, and link it in your html page

### KSbus also handle distributed use cases using a CombinedServer


## Get Started

```sh
go get github.com/kamalshkeir/ksbus@v1.0.4
```

## You don't know where you can use it ?, here is a simple use case example:
#### let's take a discord like application for example, if you goal is to broadcast message in realtime to all room members notifying them that a new user joined the room, you can do it using pooling of course, but it's not very efficient, you can use also any broker but it will be hard to subscribe from the browser or html directly

## KSbus make it very easy, here is how:

###### Client side:

```html
<script src="path/to/Bus.js"></script>
<script>
let bus = new Bus("localhost:9313");
bus.autorestart=true;
this.restartevery=5;
bus.OnOpen = () => {
    let sub = bus.Subscribe("room-client",(data,subs) => {
        // show notification
		...
		// you can use subs to unsubscribe
		subs.Unsubscribe();
    });

	// or you can unsubscribe from outside the handler using the returned sub
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
func New() *Bus
func (b *Bus) Subscribe(topic string, fn func(data map[string]any, ch Channel),name ...string) (ch Channel)
func (b *Bus) Unsubscribe(topic string,ch Channel)
func (b *Bus) Publish(topic string, data map[string]any)
func (b *Bus) RemoveTopic(topic string)
func (b *Bus) SendToNamed(name string, data map[string]any)
// in param and returned subscribe channel
func (ch Channel) Unsubscribe() Channel 
```

### Server Bus (use the internal bus and a websocket server)
```go
func NewServer() *Server
func (s *Server) JoinCombinedServer(combinedAddr string,secure bool) // not tested yet
func (s *Server) SendToServer(addr string, data map[string]any, secure ...bool) // allow you to send data to another server, and listen for it using ksbus.BeforeServersData
func (s *Server) Subscribe(topic string, fn func(data map[string]any,ch Channel),name ...string) (ch Channel) 
func (s *Server) Unsubscribe(topic string, ch Channel)
func (s *Server) Publish(topic string, data map[string]any)
func (s *Server) RemoveTopic(topic string)
func (s *Server) SendToNamed(name string, data map[string]any)
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
	
	bus.App.GET("/",func(c *kmux.Context) {
		c.Html("index.html",nil)
	})

    // if you specify a name 'go' to this subscription like below, you will receive data from any Publish on topic 'server' AND SendToNamed on 'server:go' name, so SendToNamed allow you to send not for all listeners on the topic, but the unique named one 'topic1:go'
	bus.Subscribe("server",func(data map[string]any, ch ksbus.Channel) {
		log.Println("server recv:",data)
	},"go")

	
	bus.App.GET("/pp",func(c *kmux.Context) {
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
func NewClient(addr string, secure bool,path ...string) *Client
func (client *Client) Subscribe(topic string,handler func(data map[string]any,sub *ClientSubscription), name ...string) *ClientSubscription
func (client *Client) Unsubscribe(topic string)
func (client *Client) Publish(topic string, data map[string]any) 
func (client *Client) RemoveTopic(topic string)
func (client *Client) SendToNamed(name string, data map[string]any)
func (client *Client) Run()

// param and returned subscription
func (subscribtion *ClientSubscription) Unsubscribe() (*ClientSubscription)
```

##### Example
```go
func main() {
	client:= ksbus.NewClient()
	client.AutoRestart = true
	err := client.Connect("localhost:9313",false)
	if klog.CheckError(err){
		return
	}
    // if you specify a name 'go' to this subscription like below, you will receive data from any Publish on topic 'topic1' AND any SendToNamed on 'topic1:go' name, so SendToNamed allow you to send not for all listeners on the topic, but the unique named one 'topic1:go'
	client.Subscribe("topic1",func(data map[string]any, unsub *ksbus.ClientSubscription) {
		fmt.Println("client recv",data)
	},"go")

    // this will only receive on Publish on topic 'topic2', because no name specified , so you can't send only for this one using SendToNamed
    client.Subscribe("topic2",func(data map[string]any, unsub *ksbus.ClientSubscription) {
		fmt.Println("client recv",data)
	})

	client.Run()
}
```


### Client JS
##### you can find the client ws wrapper in the repos JS folder above
```js
class Bus {
    constructor(addr,path="/ws/bus",secure=false)
    this.autorestart=false;
    this.restartevery=10; // try reconnect every 10s if autorestart enabled
OnOpen(e)
Subscribe(topic,handler,name="")
Unsubscribe(topic,name="")
Publish(topic,data)
SendToNamed(name,data,topic="")
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
let bus = new Bus("localhost:9313");
bus.autorestart=true;
this.restartevery=5;
bus.OnOpen = (e) => {
    let sub = bus.Subscribe("client",(data,subs) => {
        console.log("data:",data);
        bus.Publish("server",{
            "msg":"hello server from client js",
        })
    },"js");
}

btn.addEventListener("click",(e) => {
    e.preventDefault();
    // send to only one connection , our go client in this case, so this is a communication between client js and client go
    bus.SendToNamed("client:go",{
        "msg":"hello go client from client js"
    })
})

</script>
</body>
</html>
```


### Combined Server

```go
func NewCombinedServer(newAddress string,secure bool, serversAddrs ...AddressOption) *CombinedServer
func (s *CombinedServer) Subscribe(topic string, fn func(data map[string]any, ch Channel), name ...string) (ch Channel)
func (s *CombinedServer) Unsubscribe(topic string, ch Channel)
func (s *CombinedServer) SendToNamed(name string, data map[string]any)
func (s *CombinedServer) Publish(topic string, data map[string]any)
func (s *CombinedServer) RemoveTopic(topic string)
func (s *CombinedServer) Run()
func (s *CombinedServer) RunTLS(cert string, certKey string)
func (s *CombinedServer) RunAutoTLS(subDomains ...string)
func (s *CombinedServer) handleWS(addr string)
```

##### Examples:

###### Server 1:

```go
func main() {
	bus := ksbus.NewServer()
	ksbus.DEBUG=true
	bus.App.LocalTemplates("../../tempss")
	bus.App.LocalStatics("../../assets","/assets/")
	addr := "localhost:9313"
	bus.App.GET("/",func(c *kmux.Context) {
		c.Html("index.html",map[string]any{
			"addr":addr,
		})
	})

	bus.Subscribe("server",func(data map[string]any, ch ksbus.Channel) {
		fmt.Println(addr,"recv from",ch.Id,", data= ",data,)
	})

	
	bus.Run(addr)
}

```


###### Server 2:

```go
func main() {
	bus := ksbus.NewServer()
	ksbus.DEBUG=true
	bus.App.LocalTemplates("../../tempss")
	bus.App.LocalStatics("../../assets","/assets/")
	addr := "localhost:9314"
	bus.App.GET("/",func(c *kmux.Context) {
		c.Html("index.html",map[string]any{
			"addr":addr,
		})
	})

	bus.Subscribe("server",func(data map[string]any, ch ksbus.Channel) {
		fmt.Println(addr,"recv from",ch.Id,", data= ",data,)
	})

	
	bus.Run(addr)
}

```


###### Combined Server:

```go
func main() {
	server1Addr := ksbus.AddressOption{
		Address: "localhost:9313",
		Secure: false,
	}
	server2Addr := ksbus.AddressOption{
		Address: "localhost:9314",
		Secure: false,
	}
	bus := ksbus.NewCombinedServer("localhost:9300",false,server1Addr,server2Addr)
	bus.Subscribe("server",func(data map[string]any, ch ksbus.Channel) {
		fmt.Println("master recv data= ",data)
	})

	// what will happen here is that topics will be available in all serverBus, let's say you are subscribed to a topic in server1, you can publish on server2 or combined, both will tell server1 to publish 
	bus.Run()
}
```

## Before Handlers 
```go
// before upgrade WS connection
BeforeUpgradeWS=func(r *http.Request) bool {
	return true
}
// before recv data on WS
BeforeDataWS = func(data map[string]any,conn *ws.Conn, originalRequest *http.Request) bool {
	return true
}
// Before Recv data from another server
BeforeServersData = func(data any,conn *ws.Conn) {
	return
}
```


## Example Python Client
```sh
pip install ksbus==1.2.0
```
```py
from ksbus import Bus


# onOpen callback that let you know when connection is ready, it take the bus as param
def onOpen(bus):
    print("connected")
    # bus.autorestart=True
    # Publish publish to topic
    bus.Publish("top", {
        "data": "hello from python"
    })
    # Subscribe, it also return the subscription
    bus.Subscribe("python", pythonTopicHandler)
    # SendToNamed publish to named topic
    bus.SendToNamed("top:srv", {
        "data": "hello again from python"
    })
    # bus.Unsubscribe("python")
    print("finish everything")


# pythonTopicHandler handle topic 'python'
def pythonTopicHandler(data, subs):
    print("recv on topic python:", data)
    # Unsubscribe
    #subs.Unsubscribe()

if __name__ == "__main__":
    Bus("localhost:9313", onOpen=onOpen) # blocking
    print("prorgram exited")
```