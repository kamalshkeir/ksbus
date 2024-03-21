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
	onDataWS      func(data map[string]any, conn *ws.Conn) error
	onId          func(data map[string]any, subs *ClientSubscription)
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

type ClientConnectOptions struct {
	Id           string
	Address      string
	Secure       bool
	Path         string // default ksbus.ServerPath
	Autorestart  bool
	RestartEvery time.Duration
	OnDataWs     func(data map[string]any, conn *ws.Conn) error
	OnId         func(data map[string]any, subs *ClientSubscription) // used when client bus receive data on his ID 'client.Id'
}

var clientId string

func NewClient(opts ClientConnectOptions) (*Client, error) {
	if opts.Autorestart && opts.RestartEvery == 0 {
		opts.RestartEvery = 10 * time.Second
	}
	if opts.OnDataWs == nil {
		opts.OnDataWs = func(data map[string]any, conn *ws.Conn) error { return nil }
	}
	cl := &Client{
		Id:            opts.Id,
		Autorestart:   opts.Autorestart,
		RestartEvery:  opts.RestartEvery,
		topicHandlers: kmap.New[string, func(map[string]any, *ClientSubscription)](false),
		onDataWS:      opts.OnDataWs,
		onId:          opts.OnId,
		Done:          make(chan struct{}),
	}
	if cl.Id == "" {
		cl.Id = GenerateUUID()
	}
	clientId = cl.Id
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

	_ = c.WriteJSON(map[string]any{
		"action": "ping",
		"from":   client.Id,
	})
	client.handle()
	klog.Printfs("client connected to %s\n", u.String())
	return nil
}

func (client *Client) handle() {
	client.handleData(func(data map[string]any, sub *ClientSubscription) {
		if v, ok := data["to_id"]; ok && client.onId != nil && v.(string) == client.Id {
			delete(data, "to_id")
			client.onId(data, sub)
		}
		v1, okTopic := data["topic"]
		eventId, okEvent := data["event_id"]
		if okEvent {
			client.Publish(eventId.(string), map[string]any{
				"ok":   "done",
				"from": client.Id,
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
		"from":   id,
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
		"from":   client.Id,
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
		"from":   client.Id,
	}
	_ = client.Conn.WriteJSON(data)
}

func (client *Client) PublishToServer(addr string, data map[string]any, secure ...bool) {
	data = map[string]any{
		"action": "pub_server",
		"data":   data,
		"addr":   addr,
		"from":   client.Id,
	}
	if len(secure) > 0 && secure[0] {
		data["secure"] = true
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

func (client *Client) PublishWaitRecv(topic string, data map[string]any, onRecv func(data map[string]any), onExpire func(eventId string, topic string)) {
	eventId := GenerateUUID()
	data["from"] = client.Id
	data["event_id"] = eventId
	data["topic"] = topic
	done := make(chan struct{})

	cs := client.Subscribe(eventId, func(data map[string]any, sub *ClientSubscription) {
		done <- struct{}{}
		if onRecv != nil {
			onRecv(data)
		}
		sub.Unsubscribe()
	})
	client.Publish(topic, data)
free:
	for {
		select {
		case <-done:
			break free
		case <-time.After(500 * time.Millisecond):
			if onExpire != nil {
				onExpire(eventId, topic)
			}
			cs.Unsubscribe()
			break free
		}
	}
}

func (client *Client) PublishToIDWaitRecv(id string, data map[string]any, onRecv func(data map[string]any), onExpire func(eventId string, id string)) {
	eventId := GenerateUUID()
	data["from"] = client.Id
	data["event_id"] = eventId
	data["id"] = id
	done := make(chan struct{})

	cs := client.Subscribe(eventId, func(data map[string]any, sub *ClientSubscription) {
		done <- struct{}{}
		if onRecv != nil {
			onRecv(data)
		}
		sub.Unsubscribe()
	})
	client.PublishToID(id, data)
free:
	for {
		select {
		case <-done:
			break free
		case <-time.After(500 * time.Millisecond):
			if onExpire != nil {
				onExpire(eventId, id)
			}
			cs.Unsubscribe()
			break free
		}
	}
}

func (client *Client) RemoveTopic(topic string) {
	data := map[string]any{
		"action": "removeTopic",
		"topic":  topic,
		"from":   client.Id,
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
		"from":   clientId,
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
				err = client.onDataWS(message, client.Conn)
				if err == nil {
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
