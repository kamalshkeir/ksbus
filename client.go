package kbus

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strings"

	"github.com/kamalshkeir/klog"
	"github.com/kamalshkeir/kmux/ws"
)

type Client struct {
	id   string
	serverAddr string
	conn *ws.Conn
	done chan struct{}
}

type ClientSubscription struct {
	Id    string
	Topic string
	Name  string
	Conn  *ws.Conn
}



func NewClient(addr string, secure bool,path ...string) (*Client,error) {
	sch := "ws"
	if CLIENT_SECURE || secure {
		CLIENT_SECURE=true
		sch="wss"
	}
	spath := ServerPath
	if len(path) > 0 && path[0] != "" {
		spath=path[0]
	}
	u := url.URL{Scheme: sch, Host: addr, Path: spath}
	klog.Printf("connecting to %s\n", u.String())
	c, resp, err := ws.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		if err == ws.ErrBadHandshake {
			if DEBUG {
				klog.Printf("rdhandshake failed with status %d \n", resp.StatusCode)
			}
			return nil,err
		} 
		
		if err == ws.ErrCloseSent {
			if DEBUG {
				klog.Printf("rdserver connection closed with status %d \n", resp.StatusCode)
			}	
			return nil,err
		} else {
			if DEBUG {
				klog.Printf("rdNewClient error:%v\n", err)
			}	
			return nil,err
		}
	}
	
	client := &Client{
		id: GenerateRandomString(5),
		serverAddr: u.String(),
		conn: c,
		done: make(chan struct{}),
	}
	client.handle()
	return client,nil
}


func (client *Client) handle() {
	client.handleData(func(data map[string]any,sub *ClientSubscription) {
		v1,okTopic := data["topic"]
		v2,okName := data["name"]

		if okTopic && okName {
			if fn,ok := mClientTopicHandlers.Get(v1.(string)+":"+v2.(string));ok {
				fn(data,sub)
			} 
		} else if okTopic {
			if vv,ok := v1.(string);ok {
				if fn,ok := mClientTopicHandlers.Get(vv);ok {
					fn(data,sub)
				}
			} 
		} else if okName {
			if vv,ok := v2.(string);ok {
				if fn,ok := mClientTopicHandlers.Get(vv);ok {
					fn(data,sub)
				}
			} 
		}	 
	})
}

func (client *Client) Subscribe(topic string,handler func(data map[string]any,sub *ClientSubscription), name ...string) *ClientSubscription {
	id := client.id
	data := map[string]any{
		"action":"sub",
		"topic":topic,
		"id":id,
	}
	nn := ""
	if len(name) > 0 && name[0] != "" {
		data["name"]=name[0]
		nn=name[0]
	}
	
	err :=client.conn.WriteJSON(data)
	if err != nil {
		fmt.Println("error subscribing on",topic,err)
		return &ClientSubscription{
			Id: id,
			Topic: topic,
			Name: nn,
			Conn: client.conn,
		}
	}
	if nn != "" {
		mClientTopicHandlers.Set(topic,handler)
		mClientTopicHandlers.Set(topic+":"+nn,handler)
	} else {		
		mClientTopicHandlers.Set(topic,handler)
	}
	return &ClientSubscription{
		Id: id,
		Topic: topic,
		Name: nn,
		Conn: client.conn,
	}
}

func (client *Client) Unsubscribe(topic string) {
	data := map[string]any{
		"action":"unsub",
		"topic":topic,
		"id":client.id,
	}
	err :=client.conn.WriteJSON(data)
	if err != nil {
		fmt.Println("error unsubscribing on",topic,err,data)
		return
	}
}

func (client *Client) Publish(topic string, data map[string]any) {
	data = map[string]any{
		"data":data,
		"action":"pub",
		"topic":topic,
		"id":client.id,
	}
	_ =client.conn.WriteJSON(data)
}

func (client *Client) RemoveTopic(topic string) {
	data := map[string]any{
		"action":"removeTopic",
		"topic":topic,
		"id":client.id,
	}
	err :=client.conn.WriteJSON(data)
	if err != nil {
		fmt.Println("error RemoveTopic, data:",data,", err:",err)
		return 
	}
	mClientTopicHandlers.Delete(topic)
}

func (client *Client) SendTo(name string, data map[string]any)  {
	data = map[string]any{
		"action":"sendTo",
		"name":name,
		"data":data,
		"id":client.id,
	}
	err :=client.conn.WriteJSON(data)
	if err != nil {
		fmt.Println("error SendTo, data:",data,", err:",err)
		return 
	}
}

func (client *Client) sendDataToServer(addr string, data map[string]any)  {
	data["id"]=client.id
	err :=client.conn.WriteJSON(data)
	if err != nil {
		fmt.Println("error SendTo, data:",data,", err:",err)
		return 
	}
}

func (client *Client) Run() {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	for {
		select {
		case <-client.done:
			return
		case <-interrupt:
			log.Println("interrupt")
			// Cleanly close the connection by sending a close message and then
			// waiting (with timeout) for the server to close the connection.
			err := client.conn.WriteMessage(ws.CloseMessage, ws.FormatCloseMessage(ws.CloseNormalClosure, ""))
			if err != nil {
				log.Println("write close:", err)
				return
			}			
			_=client.conn.Close() 
			client.conn=nil
			<-client.done
			return
		}
	}
}

func (subscribtion *ClientSubscription) Unsubscribe() *ClientSubscription {
	data := map[string]any{
		"action":"unsub",
		"topic":subscribtion.Topic,
		"name":subscribtion.Name,
		"id":subscribtion.Id,
	}
	err :=subscribtion.Conn.WriteJSON(data)
	if err != nil {
		fmt.Println("error unsubscribing on",subscribtion.Topic,err,data)
		return subscribtion
	}
	return subscribtion
}


func (client *Client) handleData(fn func(data map[string]any,sub *ClientSubscription)) {
	go func() {
		defer close(client.done)
		for {
			message := map[string]any{}
			if client.conn != nil {
				err := client.conn.ReadJSON(&message)
				if err == ws.ErrCloseSent || strings.Contains(err.Error(),"use of closed network connection") {
					klog.Printfs("rdOnData error:%v\n", err)
					return
				} else if err != nil {
					klog.Printfs("rdOnData error:%v\n", err)
					return
				}
				if DEBUG {
					klog.Printf("client recv: %v\n",message)
				}
				contin := BeforeDataWS(message,client.conn,nil)
				if contin {
					sub := ClientSubscription{
						Conn: client.conn,
					}
					if v,ok := message["topic"];ok {
						sub.Topic=v.(string)
					} 
					if v,ok := message["name"];ok {
						sub.Name=v.(string)
					}
					fn(message,&sub)
				}
			}
		}
	}()
}