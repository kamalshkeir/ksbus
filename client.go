package ksbus

import (
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
	onData        func(data map[string]any)
	RestartEvery  time.Duration
	Conn          *ws.Conn
	Autorestart   bool
	Done          chan struct{}
	topicHandlers *kmap.SafeMap[string, func(map[string]any, *ClientSubscription)]
}

type ClientSubscription struct {
	Id    string
	Topic string
	Conn  *ws.Conn
}

var clientId string

type ClientConnectOptions struct {
	Address      string
	Secure       bool
	Path         string // default ksbus.ServerPath
	Autorestart  bool
	RestartEvery time.Duration
	OnData       func(data map[string]any)
}

func NewClient(opts ClientConnectOptions) (*Client, error) {
	clientId = GenerateUUID()
	if opts.Autorestart && opts.RestartEvery == 0 {
		opts.RestartEvery = 10 * time.Second
	}
	cl := &Client{
		Id:            clientId,
		Autorestart:   opts.Autorestart,
		RestartEvery:  opts.RestartEvery,
		topicHandlers: kmap.New[string, func(map[string]any, *ClientSubscription)](false),
		onData:        opts.OnData,
		Done:          make(chan struct{}),
	}
	err := cl.connect(opts)
	if klog.CheckError(err) {
		return nil, err
	}
	return cl, nil
}

func (client *Client) connect(opts ClientConnectOptions) error {
	sch := "ws"
	if CLIENT_SECURE || opts.Secure {
		CLIENT_SECURE = true
		sch = "wss"
	}
	spath := ServerPath
	if opts.Path != "" {
		spath = opts.Path
	}
	u := url.URL{Scheme: sch, Host: opts.Address, Path: spath}
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
		if client.onData != nil {
			client.onData(data)
		}
		v1, okTopic := data["topic"]
		eventId, okEvent := data["event_id"]
		if okEvent {
			client.Publish(eventId.(string), map[string]any{
				"success": "got the event",
				"id":      client.Id,
			})
		}
		found := false
		if okTopic {
			if vv, ok := v1.(string); ok {
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

func (client *Client) Subscribe(topic string, handler func(data map[string]any, sub *ClientSubscription)) *ClientSubscription {
	id := client.Id
	data := map[string]any{
		"action": "sub",
		"topic":  topic,
		"id":     id,
	}

	err := client.Conn.WriteJSON(data)
	if err != nil {
		klog.Printf("rderror subscribing on %s %v\n", topic, err)
		return &ClientSubscription{
			Id:    id,
			Topic: topic,
			Conn:  client.Conn,
		}
	}
	client.topicHandlers.Set(topic, handler)
	return &ClientSubscription{
		Id:    id,
		Topic: topic,
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
		klog.Printf("rderror unsubscribing on %s %v %v\n", topic, err, data)
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

func (client *Client) PublishToID(id string, data map[string]any) {
	data = map[string]any{
		"data":   data,
		"action": "pub_id",
		"id":     id,
		"from":   client.Id,
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
		klog.Printf("rderror RemoveTopic, data: %v err :%v\n", data, err)
		return
	}
	client.topicHandlers.Delete(topic)
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
			klog.Printf("rdinterrupt\n")
			client.Close()
			return
		}
	}
}

func (subscribtion *ClientSubscription) Unsubscribe() *ClientSubscription {
	data := map[string]any{
		"action": "unsub",
		"topic":  subscribtion.Topic,
		"id":     clientId,
	}
	err := subscribtion.Conn.WriteJSON(data)
	if err != nil {
		klog.Printf("rderror unsubscribing on %s %v %v\n", subscribtion.Topic, err, data)
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
				contin := OnDataWS(message, client.Conn, nil)
				if contin {
					sub := ClientSubscription{
						Conn: client.Conn,
					}
					if v, ok := message["topic"]; ok {
						sub.Topic = v.(string)
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
