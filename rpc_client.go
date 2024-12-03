package ksbus

import (
	"errors"
	"net/rpc"
	"os"
	"os/signal"
	"time"

	"encoding/gob"

	"github.com/kamalshkeir/kmap"
	"github.com/kamalshkeir/lg"
)

// RPCClient implements a client that connects to the bus system using RPC
type RPCClient struct {
	Id            string
	ServerAddr    string
	conn          *rpc.Client
	topicHandlers *kmap.SafeMap[string, func(map[string]any, RPCSubscriber)]
	onId          func(data map[string]any, unsub RPCSubscriber)
	onDataRPC     func(data map[string]any) error
	onClose       func()
	Autorestart   bool
	RestartEvery  time.Duration
	Done          chan struct{}
}

// RPCSubscriber represents a subscription to a topic via RPC
type RPCSubscriber struct {
	client *RPCClient
	Id     string
	Topic  string
}

func (s RPCSubscriber) Unsubscribe() {
	s.client.Unsubscribe(s.Topic)
}

type RPCClientOptions struct {
	Id           string
	Address      string // RPC server address (e.g. "localhost:9314")
	OnId         func(data map[string]any, unsub RPCSubscriber)
	OnDataRPC    func(data map[string]any) error
	OnClose      func()
	Autorestart  bool
	RestartEvery time.Duration
}

// RPCRequest represents the data structure for RPC calls
type RPCRequest struct {
	Action string
	Topic  string
	Data   map[string]any
	From   string
	Id     string
}

// RPCResponse represents the response from RPC calls
type RPCResponse struct {
	Data  map[string]any
	Error string
}

// NewRPCClient creates a new RPC client connection to the bus
func NewRPCClient(opts RPCClientOptions) (*RPCClient, error) {
	// Register types for gob encoding
	gob.Register(map[string]interface{}{})

	if opts.Id == "" {
		opts.Id = GenerateUUID()
	}
	if opts.Autorestart && opts.RestartEvery == 0 {
		opts.RestartEvery = 10 * time.Second
	}
	if opts.OnDataRPC == nil {
		opts.OnDataRPC = func(data map[string]any) error { return nil }
	}

	client := &RPCClient{
		Id:            opts.Id,
		ServerAddr:    opts.Address,
		topicHandlers: kmap.New[string, func(map[string]any, RPCSubscriber)](),
		onId:          opts.OnId,
		onDataRPC:     opts.OnDataRPC,
		onClose:       opts.OnClose,
		Autorestart:   opts.Autorestart,
		RestartEvery:  opts.RestartEvery,
		Done:          make(chan struct{}),
	}

	// Connect to RPC server
	err := client.connect()
	if err != nil {
		return nil, err
	}

	// Start listening for messages
	go client.listen()

	return client, nil
}

func (c *RPCClient) connect() error {
	conn, err := rpc.DialHTTP("tcp", c.ServerAddr)
	if err != nil {
		if c.Autorestart {
			lg.Info("Connection failed, will retry in", "seconds", c.RestartEvery.Seconds())
			time.Sleep(c.RestartEvery)
			err := c.connect()
			if err != nil {
				lg.Error("Failed to reconnect", "err", err)
				return err
			}
			lg.Info("Successfully reconnected")
			return nil
		}
		return err
	}
	c.conn = conn

	// Initial ping to register client
	err = c.ping()
	if err != nil {
		if c.Autorestart {
			lg.Info("Ping failed, will retry in", "seconds", c.RestartEvery.Seconds())
			time.Sleep(c.RestartEvery)
			err := c.connect()
			if err != nil {
				lg.Error("Failed to reconnect", "err", err)
				return err
			}
			lg.Info("Successfully reconnected")
			return nil
		}
		return err
	}

	return nil
}

func (c *RPCClient) ping() error {
	req := RPCRequest{
		Action: "ping",
		From:   c.Id,
	}
	var resp RPCResponse
	err := c.conn.Call("BusRPC.Ping", req, &resp)
	if err != nil {
		return err
	}
	if resp.Error != "" {
		return errors.New(resp.Error)
	}
	return nil
}

func (c *RPCClient) Subscribe(topic string, handler func(data map[string]any, unsub RPCSubscriber)) RPCSubscriber {
	req := RPCRequest{
		Action: "sub",
		Topic:  topic,
		From:   c.Id,
	}
	var resp RPCResponse
	err := c.conn.Call("BusRPC.Subscribe", req, &resp)
	if err != nil {
		lg.Error("error subscribing", "topic", topic, "err", err)
		return RPCSubscriber{
			client: c,
			Id:     c.Id,
			Topic:  topic,
		}
	}

	c.topicHandlers.Set(topic, handler)
	return RPCSubscriber{
		client: c,
		Id:     c.Id,
		Topic:  topic,
	}
}

func (c *RPCClient) Unsubscribe(topic string) {
	req := RPCRequest{
		Action: "unsub",
		Topic:  topic,
		From:   c.Id,
	}
	var resp RPCResponse
	err := c.conn.Call("BusRPC.Unsubscribe", req, &resp)
	if err != nil {
		lg.Error("error unsubscribing", "topic", topic, "err", err)
		return
	}
	c.topicHandlers.Delete(topic)
}

func (c *RPCClient) Publish(topic string, data map[string]any) {
	req := RPCRequest{
		Action: "pub",
		Topic:  topic,
		Data:   data,
		From:   c.Id,
	}
	var resp RPCResponse
	err := c.conn.Call("BusRPC.Publish", req, &resp)
	if err != nil {
		lg.Error("error publishing", "topic", topic, "err", err)
	}
}

func (c *RPCClient) PublishToID(id string, data map[string]any) {
	req := RPCRequest{
		Action: "pub_id",
		Id:     id,
		Data:   data,
		From:   c.Id,
	}
	var resp RPCResponse
	err := c.conn.Call("BusRPC.PublishToID", req, &resp)
	if err != nil {
		lg.Error("error publishing to ID", "id", id, "err", err)
	}
}

func (c *RPCClient) PublishWaitRecv(topic string, data map[string]any, onRecv func(data map[string]any), onExpire func(eventId string, topic string)) {
	eventId := GenerateUUID()
	data["from"] = c.Id
	data["event_id"] = eventId
	data["topic"] = topic
	done := make(chan struct{})

	sub := c.Subscribe(eventId, func(data map[string]any, unsub RPCSubscriber) {
		done <- struct{}{}
		if onRecv != nil {
			onRecv(data)
		}
		unsub.Unsubscribe()
	})

	c.Publish(topic, data)

free:
	for {
		select {
		case <-done:
			break free
		case <-time.After(500 * time.Millisecond):
			if onExpire != nil {
				onExpire(eventId, topic)
			}
			sub.Unsubscribe()
			break free
		}
	}
}

func (c *RPCClient) PublishToIDWaitRecv(id string, data map[string]any, onRecv func(data map[string]any), onExpire func(eventId string, id string)) {
	eventId := GenerateUUID()
	data["from"] = c.Id
	data["event_id"] = eventId
	data["id"] = id
	done := make(chan struct{})

	sub := c.Subscribe(eventId, func(data map[string]any, unsub RPCSubscriber) {
		done <- struct{}{}
		if onRecv != nil {
			onRecv(data)
		}
		unsub.Unsubscribe()
	})

	c.PublishToID(id, data)

free:
	for {
		select {
		case <-done:
			break free
		case <-time.After(500 * time.Millisecond):
			if onExpire != nil {
				onExpire(eventId, id)
			}
			sub.Unsubscribe()
			break free
		}
	}
}

func (c *RPCClient) RemoveTopic(topic string) {
	req := RPCRequest{
		Action: "removeTopic",
		Topic:  topic,
		From:   c.Id,
	}
	var resp RPCResponse
	err := c.conn.Call("BusRPC.RemoveTopic", req, &resp)
	if err != nil {
		lg.Error("error removing topic", "topic", topic, "err", err)
	}
}

func (c *RPCClient) Close() error {
	if c.onClose != nil {
		c.onClose()
	}
	return c.conn.Close()
}

// OnClose sets the callback function to be called when the connection is closed
func (c *RPCClient) OnClose(fn func()) {
	c.onClose = fn
}

// listen continuously polls for messages from the server
func (c *RPCClient) listen() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-c.Done:
			return
		case <-ticker.C:
			req := RPCRequest{
				Action: "poll",
				From:   c.Id,
			}
			var resp RPCResponse
			err := c.conn.Call("BusRPC.Poll", req, &resp)
			if err != nil {
				if errors.Is(err, rpc.ErrShutdown) {
					lg.ErrorC(err.Error())
					if c.onClose != nil {
						c.onClose()
					}
					if c.Autorestart {
						lg.Info("Connection lost, attempting to reconnect in", "seconds", c.RestartEvery.Seconds())
						time.Sleep(c.RestartEvery)
						if err := c.connect(); err != nil {
							lg.Error("Failed to reconnect", "err", err)
							continue
						}
						lg.Info("Successfully reconnected")
						continue
					}
					c.Close()
					return
				}
				lg.Error("error polling messages", "err", err)
				continue
			}

			if len(resp.Data) > 0 {
				c.handleMessage(resp.Data)
			}
		}
	}
}

func (c *RPCClient) handleMessage(data map[string]any) {
	// Check if message is for a topic we're no longer subscribed to
	if topic, ok := data["topic"].(string); ok {
		if _, exists := c.topicHandlers.Get(topic); !exists {
			// Skip processing messages for topics we're not subscribed to
			return
		}
	}

	// Call general handler first for all messages
	if err := c.onDataRPC(data); err != nil {
		lg.Error("error handling RPC data", "err", err)
	}

	if eventID, ok := data["event_id"]; ok {
		c.Publish(eventID.(string), map[string]any{
			"ok":   "done",
			"from": c.Id,
		})
	}

	if toID, ok := data["to_id"]; ok && c.onId != nil && toID.(string) == c.Id {
		delete(data, "to_id")
		sub := RPCSubscriber{
			client: c,
			Id:     c.Id,
		}
		c.onId(data, sub)
		return
	}

	if topic, ok := data["topic"].(string); ok {
		if handler, exists := c.topicHandlers.Get(topic); exists {
			sub := RPCSubscriber{
				client: c,
				Id:     c.Id,
				Topic:  topic,
			}
			handler(data, sub)
			return
		}
	}
}

func (c *RPCClient) Run() {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	for {
		select {
		case <-c.Done:
			return
		case <-interrupt:
			lg.Info("RPC Closed")
			c.Close()
			return
		}
	}
}
