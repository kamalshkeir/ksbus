package ksbus

import (
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/kamalshkeir/kmap"
	"github.com/kamalshkeir/ksmux/ws"
	"github.com/kamalshkeir/lg"
)

type Client struct {
	Id            string
	ServerAddr    string
	onDataWS      func(data map[string]any, conn *ws.Conn) error
	onId          func(data map[string]any, unsub Unsub)
	RestartEvery  time.Duration
	Conn          *ws.Conn
	Autorestart   bool
	Done          chan struct{}
	topicHandlers *kmap.SafeMap[string, func(map[string]any, Unsub)]
}

type ClientConnectOptions struct {
	Id           string
	Address      string
	Secure       bool
	Path         string // default ksbus.ServerPath
	Autorestart  bool
	RestartEvery time.Duration
	OnDataWs     func(data map[string]any, conn *ws.Conn) error
	OnId         func(data map[string]any, unsub Unsub) // used when client bus receive data on his ID 'client.Id'
}

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
		topicHandlers: kmap.New[string, func(map[string]any, Unsub)](),
		onDataWS:      opts.OnDataWs,
		onId:          opts.OnId,
		Done:          make(chan struct{}),
	}
	if cl.Id == "" {
		cl.Id = GenerateUUID()
	}
	err := cl.connect(opts)
	if lg.CheckError(err) {
		return nil, err
	}
	return cl, nil
}

func (client *Client) connect(opts ClientConnectOptions) error {
	sch := "ws"
	if opts.Secure {
		sch = "wss"
	}
	spath := ""
	if opts.Path != "" {
		spath = opts.Path
	} else {
		spath = "/ws/bus"
	}
	u := url.URL{Scheme: sch, Host: opts.Address, Path: spath}
	client.ServerAddr = u.String()
	c, resp, err := ws.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		if client.Autorestart {
			lg.Info("Restarting")
			time.Sleep(client.RestartEvery)
			RestartSelf()
		}
		if err == ws.ErrBadHandshake {
			lg.DebugC("handshake failed with status", "status", resp.StatusCode)
			return err
		}

		if err == ws.ErrCloseSent {
			lg.DebugC("server connection closed with status", "status", resp.StatusCode)
			return err
		} else {
			lg.DebugC("NewClient error", "err", err)
			return err
		}
	}
	client.Conn = c

	_ = c.WriteJSON(map[string]any{
		"action": "ping",
		"from":   client.Id,
	})
	client.handle()
	lg.Printfs("client connected to %s\n", u.String())

	return nil
}

func (client *Client) handle() {
	client.handleData(func(data map[string]any, sub *Subscriber) {
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
		if !found {
			lg.ErrorC("client handler for topic not found", "topic", v1)
		}
	})
}

func (client *Client) Subscribe(topic string, handler func(data map[string]any, unsub Unsub)) Unsub {
	id := client.Id
	data := map[string]any{
		"action": "sub",
		"topic":  topic,
		"from":   id,
	}

	err := client.Conn.WriteJSON(data)
	if err != nil {
		lg.Error("error subscribing", "topic", topic, "err", err)
		return &Subscriber{
			Id:    id,
			Topic: topic,
			Conn:  client.Conn,
		}
	}
	client.topicHandlers.Set(topic, handler)
	return &Subscriber{
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
		lg.Error("error unsub", "topic", topic, "err", err, "data", data)
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

	cs := client.Subscribe(eventId, func(data map[string]any, unsub Unsub) {
		done <- struct{}{}
		if onRecv != nil {
			onRecv(data)
		}
		unsub.Unsubscribe()
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

	cs := client.Subscribe(eventId, func(data map[string]any, unsub Unsub) {
		done <- struct{}{}
		if onRecv != nil {
			onRecv(data)
		}
		unsub.Unsubscribe()
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
		lg.ErrorC("error RemoveTopic", "err", err, "data", data)
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
			lg.Info("Closed")
			client.Close()
			return
		}
	}
}

func (client *Client) handleData(fn func(data map[string]any, sub *Subscriber)) {
	go func() {
		defer close(client.Done)
		for {
			message := map[string]any{}
			if client.Conn != nil {
				err := client.Conn.ReadJSON(&message)
				if err != nil && (err == ws.ErrCloseSent || strings.Contains(err.Error(), "forcibly closed")) {
					if client.Autorestart {
						lg.Printfs("grRestarting\n")
						time.Sleep(client.RestartEvery)
						RestartSelf()
					} else {
						lg.Printfs("rdClosed connection error:%v\n", err)
						return
					}
				} else if err != nil {
					lg.Printfs("rdOnData error:%v\n", err)
					return
				}
				err = client.onDataWS(message, client.Conn)
				if err == nil {
					sub := Subscriber{
						Conn: client.Conn,
					}
					if v, ok := message["topic"]; ok {
						sub.Topic = v.(string)
					}
					fn(message, &sub)
				}
			} else {
				lg.Printfs("rdhandleData error: no connection\n")
				return
			}
		}
	}()
}
