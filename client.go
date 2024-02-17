package ksbus

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/kamalshkeir/klog"
	"github.com/kamalshkeir/kmap"
	"github.com/kamalshkeir/ksmux/ws"
)

type Client struct {
	Id            string
	ServerAddr    string
	RestartEvery  time.Duration
	Conn          *ws.Conn
	Autorestart   bool
	Done          chan struct{}
	topicHandlers *kmap.SafeMap[string, func(map[string]any, *ClientSubscription)]
}

type ClientSubscription struct {
	Id    string
	Topic string
	Name  string
	Conn  *ws.Conn
}

var clientId string

func NewClient() *Client {
	clientId = GenerateRandomString(10)
	return &Client{
		Id:            clientId,
		Autorestart:   false,
		RestartEvery:  10 * time.Second,
		topicHandlers: kmap.New[string, func(map[string]any, *ClientSubscription)](false),
		Done:          make(chan struct{}),
	}
}

func (client *Client) Connect(addr string, secure bool, path ...string) error {
	sch := "ws"
	if CLIENT_SECURE || secure {
		CLIENT_SECURE = true
		sch = "wss"
	}
	spath := ServerPath
	if len(path) > 0 && path[0] != "" {
		spath = path[0]
	}
	u := url.URL{Scheme: sch, Host: addr, Path: spath}
	client.ServerAddr = u.String()
	c, resp, err := ws.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		if client.Autorestart {
			klog.Printfs("grRestarting\n")
			time.Sleep(client.RestartEvery)
			RestartSelf()
		}
		if err == ws.ErrBadHandshake {
			if DEBUG {
				klog.Printf("rdhandshake failed with status %d \n", resp.StatusCode)
			}
			return err
		}

		if err == ws.ErrCloseSent {
			if DEBUG {
				klog.Printf("rdserver connection closed with status %d \n", resp.StatusCode)
			}
			return err
		} else {
			if DEBUG {
				klog.Printf("rdNewClient error:%v\n", err)
			}
			return err
		}
	}
	client.Conn = c
	client.handle()
	klog.Printfs("client connected to %s\n", u.String())
	return nil
}

func (client *Client) handle() {
	client.handleData(func(data map[string]any, sub *ClientSubscription) {
		v1, okTopic := data["topic"]
		v2, okName := data["name"]
		eventId, okEvent := data["event_id"]
		if okEvent {
			client.Publish(eventId.(string), map[string]any{
				"success": "got the event",
				"id":      client.Id,
			})
		}
		found := false
		if okTopic && okName {
			if fn, ok := client.topicHandlers.Get(v1.(string) + ":" + v2.(string)); ok {
				found = true
				fn(data, sub)
			}
		} else if okTopic {
			if vv, ok := v1.(string); ok {
				if fn, ok := client.topicHandlers.Get(vv); ok {
					found = true
					fn(data, sub)
				}
			}
		} else if okName {
			if vv, ok := v2.(string); ok {
				if fn, ok := client.topicHandlers.Get(vv); ok {
					found = true
					fn(data, sub)
				}
			}
		}
		if !found && DEBUG {
			klog.Printfs("rdclient handler for topic %s not found \n", v1)
		}
	})
}

func (client *Client) Subscribe(topic string, handler func(data map[string]any, sub *ClientSubscription), name ...string) *ClientSubscription {
	id := client.Id
	data := map[string]any{
		"action": "sub",
		"topic":  topic,
		"id":     id,
	}
	nn := ""
	if len(name) > 0 && name[0] != "" {
		data["name"] = name[0]
		nn = name[0]
	}

	err := client.Conn.WriteJSON(data)
	if err != nil {
		fmt.Println("error subscribing on", topic, err)
		return &ClientSubscription{
			Id:    id,
			Topic: topic,
			Name:  nn,
			Conn:  client.Conn,
		}
	}
	if nn != "" {
		client.topicHandlers.Set(topic, handler)
		client.topicHandlers.Set(topic+":"+nn, handler)
	} else {
		client.topicHandlers.Set(topic, handler)
	}
	return &ClientSubscription{
		Id:    id,
		Topic: topic,
		Name:  nn,
		Conn:  client.Conn,
	}
}

func (client *Client) Unsubscribe(topic string) {
	data := map[string]any{
		"action": "unsub",
		"topic":  topic,
		"id":     client.Id,
	}
	err := client.Conn.WriteJSON(data)
	if err != nil {
		fmt.Println("error unsubscribing on", topic, err, data)
		return
	}
}

func (client *Client) Publish(topic string, data map[string]any) {
	data = map[string]any{
		"data":   data,
		"action": "pub",
		"topic":  topic,
		"id":     client.Id,
	}
	_ = client.Conn.WriteJSON(data)
}

func (client *Client) RemoveTopic(topic string) {
	data := map[string]any{
		"action": "removeTopic",
		"topic":  topic,
		"id":     client.Id,
	}
	err := client.Conn.WriteJSON(data)
	if err != nil {
		fmt.Println("error RemoveTopic, data:", data, ", err:", err)
		return
	}
	client.topicHandlers.Delete(topic)
}

func (client *Client) SendToNamed(name string, data map[string]any) {
	data = map[string]any{
		"action": "sendTo",
		"name":   name,
		"data":   data,
		"id":     client.Id,
	}
	err := client.Conn.WriteJSON(data)
	if err != nil {
		klog.Printfs("error SendToNamed, data: %v, err: %v\n", data, err)
		return
	}
}

func (client *Client) sendDataToServer(data map[string]any) {
	data["id"] = client.Id
	err := client.Conn.WriteJSON(data)
	if err != nil {
		fmt.Println("error SendToNamed, data:", data, ", err:", err)
		return
	}
}

func (client *Client) Close() error {
	err := client.Conn.WriteMessage(ws.CloseMessage, ws.FormatCloseMessage(ws.CloseNormalClosure, ""))
	if err != nil {
		return err
	}
	err = client.Conn.Close()
	if err != nil {
		return err
	}
	client.Conn = nil
	<-client.Done
	return nil
}

func (client *Client) Run() {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	for {
		select {
		case <-client.Done:
			return
		case <-interrupt:
			log.Println("interrupt")
			client.Close()
			return
		}
	}
}

func (subscribtion *ClientSubscription) Unsubscribe() *ClientSubscription {
	data := map[string]any{
		"action": "unsub",
		"topic":  subscribtion.Topic,
		"name":   subscribtion.Name,
		"id":     clientId,
	}
	err := subscribtion.Conn.WriteJSON(data)
	if err != nil {
		fmt.Println("error unsubscribing on", subscribtion.Topic, err, data)
		return subscribtion
	}
	return subscribtion
}

func (client *Client) handleData(fn func(data map[string]any, sub *ClientSubscription)) {
	go func() {
		defer close(client.Done)
		for {
			message := map[string]any{}
			if client.Conn != nil {
				err := client.Conn.ReadJSON(&message)
				if err != nil && (err == ws.ErrCloseSent || strings.Contains(err.Error(), "forcibly closed")) {
					if client.Autorestart {
						klog.Printfs("grRestarting\n")
						time.Sleep(client.RestartEvery)
						RestartSelf()
					} else {
						klog.Printfs("rdClosed connection error:%v\n", err)
						return
					}
				} else if err != nil {
					klog.Printfs("rdOnData error:%v\n", err)
					return
				}
				if DEBUG {
					klog.Printfs("client handleData recv: %v\n", message)
				}
				contin := BeforeDataWS(message, client.Conn, nil)
				if contin {
					sub := ClientSubscription{
						Conn: client.Conn,
					}
					if v, ok := message["topic"]; ok {
						sub.Topic = v.(string)
					}
					if v, ok := message["name"]; ok {
						sub.Name = v.(string)
					}
					fn(message, &sub)
				}
			} else {
				klog.Printfs("rdhandleData error: no connection\n")
				return
			}
		}
	}()
}
