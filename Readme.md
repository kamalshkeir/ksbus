# Kbus is your easiest way to share/synchronise data between your Golang servers or/and between you servers and browsers(JS client) and/or between your clients, any golang applications can communicate between each others and with html pages using the JS client(Pure JS no need for node or others) with zero configuration.

# Kbus also handle distributed use cases or loadbalancing bus using a CombinedServer


##### you can handle authentication or access to certain topics using to [Before Handlers](#before-handlers)

##### Python client will be added soon 

### Internal Bus (No websockets, use channels to handle topic communications)

```go
func New() *Bus
func (b *Bus) Subscribe(topic string, fn func(data map[string]any, ch Channel),name ...string) (ch Channel)
func (b *Bus) Unsubscribe(topic string,ch Channel)
func (b *Bus) Publish(topic string, data map[string]any)
func (b *Bus) RemoveTopic(topic string)
func (b *Bus) SendTo(name string, data map[string]any)
// in param and returned subscribe channel
func (ch Channel) Unsubscribe() Channel 
```

### Server Bus (use the internal bus and a websocket server)
```go
func NewServer() *Server

func (s *Server) Subscribe(topic string, fn func(data map[string]any,ch Channel),name ...string) (ch Channel) 
func (s *Server) Unsubscribe(topic string, ch Channel)
func (s *Server) Publish(topic string, data map[string]any)
func (s *Server) RemoveTopic(topic string)
func (s *Server) SendTo(name string, data map[string]any)
func (s *Server) Run(addr string)
func (s *Server) RunTLS(addr string, cert string, certKey string)
func (s *Server) RunAutoTLS(domainName string, subDomains ...string)

// param and returned subscribe channel 
func (ch Channel) Unsubscribe() Channel
```

##### Example:
```go
func main() {
	bus := kbus.NewServer()
	bus.App.LocalTemplates("tempss") // load template folder to be used with c.HTML
	bus.App.LocalStatics("assets","/assets/")
	
	bus.App.GET("/",func(c *kmux.Context) {
		c.Html("index.html",nil)
	})

    // if you specify a name 'go' to this subscription like below, you will receive data from any Publish on topic 'server' AND SendTo on 'server:go' name, so SendTo allow you to send not for all listeners on the topic, but the unique named one 'topic1:go'
	bus.Subscribe("server",func(data map[string]any, ch kbus.Channel) {
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
func (client *Client) SendTo(name string, data map[string]any)
func (client *Client) Run()

// param and returned subscription
func (subscribtion *ClientSubscription) Unsubscribe() (*ClientSubscription)
```

##### Example
```go
func main() {
	client,_ := kbus.NewClient("localhost:9313",false)

    // if you specify a name 'go' to this subscription like below, you will receive data from any Publish on topic 'topic1' AND any SendTo on 'topic1:go' name, so SendTo allow you to send not for all listeners on the topic, but the unique named one 'topic1:go'
	client.Subscribe("topic1",func(data map[string]any, unsub *kbus.ClientSubscription) {
		fmt.Println("client recv",data)
	},"go")

    // this will only receive on Publish on topic 'topic2', because no name specified , so you can't send only for this one using SendTo
    client.Subscribe("topic2",func(data map[string]any, unsub *kbus.ClientSubscription) {
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
SendTo(name,data,topic="")
RemoveTopic(topic)
```

##### JS Client Example:

```js
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
    bus.SendTo("client:go",{
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
func (s *CombinedServer) SendTo(name string, data map[string]any)
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
	bus := kbus.NewServer()
	kbus.DEBUG=true
	bus.App.LocalTemplates("../../tempss")
	bus.App.LocalStatics("../../assets","/assets/")
	addr := "localhost:9313"
	bus.App.GET("/",func(c *kmux.Context) {
		c.Html("index.html",map[string]any{
			"addr":addr,
		})
	})

	bus.Subscribe("server",func(data map[string]any, ch kbus.Channel) {
		fmt.Println(addr,"recv from",ch.Id,", data= ",data,)
	})

	
	bus.Run(addr)
}

```


###### Server 2:

```go
func main() {
	bus := kbus.NewServer()
	kbus.DEBUG=true
	bus.App.LocalTemplates("../../tempss")
	bus.App.LocalStatics("../../assets","/assets/")
	addr := "localhost:9314"
	bus.App.GET("/",func(c *kmux.Context) {
		c.Html("index.html",map[string]any{
			"addr":addr,
		})
	})

	bus.Subscribe("server",func(data map[string]any, ch kbus.Channel) {
		fmt.Println(addr,"recv from",ch.Id,", data= ",data,)
	})

	
	bus.Run(addr)
}

```


###### Combined Server:

```go
func main() {
	server1Addr := kbus.AddressOption{
		Address: "localhost:9313",
		Secure: false,
		LoadBalanced: true, // this server will publishOnly whatever it receive to :9300 combined server
	}
	server2Addr := kbus.AddressOption{
		Address: "localhost:9314",
		Secure: false,
		Distributed: true, // this server will continue handling his own subscriptions on topics and share any action received with :9300 combined server
	}
	bus := kbus.NewCombinedServer("localhost:9300",false,server1Addr,server2Addr)
	bus.Subscribe("server",func(data map[string]any, ch kbus.Channel) {
		fmt.Println("master recv data= ",data)
	})
	bus.Run()
}
```

## Before Handlers 
```go
kbus.BeforeUpgradeWS=func(r *http.Request) bool {
	
}
kbus.BeforeDataWS=func(data map[string]any, conn *ws.Conn, originalRequest *http.Request) bool {
	
}
```