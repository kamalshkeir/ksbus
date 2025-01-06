package ksbus

import (
	"fmt"
	"net"
	"net/http"
	"net/rpc"
	"net/url"
	"time"

	"encoding/gob"

	"github.com/kamalshkeir/kmap"
	"github.com/kamalshkeir/ksmux"
	"github.com/kamalshkeir/ksmux/ws"
	"github.com/kamalshkeir/lg"
)

type Server struct {
	ID                      string
	Address                 string
	Path                    string
	Bus                     *Bus
	App                     *ksmux.Router
	busMidws                []func(ksmux.Handler) ksmux.Handler
	onWsClose               func(connID string)
	onDataWS                func(data map[string]any, conn *ws.Conn, originalRequest *http.Request) error
	onServerData            []func(data any, conn *ws.Conn)
	onId                    func(data map[string]any)
	sendToServerConnections *kmap.SafeMap[string, *ws.Conn]
	beforeUpgradeWs         func(r *http.Request) bool
	rpcServer               *rpc.Server
	idConnRPC               *kmap.SafeMap[string, *RPCConn]
	rpcMaxQueueSize         int
}

type RPCConn struct {
	Id      string
	msgChan chan map[string]any
}

type WithRpc struct {
}

type ServerOpts struct {
	ID              string
	Address         string
	BusPath         string
	BusMidws        []func(ksmux.Handler) ksmux.Handler
	OnWsClose       func(connID string)
	OnDataWS        func(data map[string]any, conn *ws.Conn, originalRequest *http.Request) error
	OnServerData    []func(data any, conn *ws.Conn)
	OnId            func(data map[string]any)
	OnUpgradeWs     func(r *http.Request) bool
	WithRPCAddress  string
	WithOtherRouter *ksmux.Router
	WithOtherBus    *Bus
}

func NewDefaultServerOptions() ServerOpts {
	return ServerOpts{
		ID:              GenerateUUID(),
		Address:         "localhost:9313",
		BusPath:         "/ws/bus",
		WithOtherRouter: ksmux.New(),
		WithOtherBus:    New(),
		OnUpgradeWs:     func(r *http.Request) bool { return true },
	}
}

func NewServer(options ...ServerOpts) *Server {
	var opts ServerOpts
	if len(options) == 0 {
		opts = NewDefaultServerOptions()
	} else {
		opts = options[0]
	}
	if opts.ID == "" {
		opts.ID = GenerateUUID()
	}
	if opts.Address == "" {
		opts.Address = "localhost:9313"
	}
	if opts.BusPath == "" {
		opts.BusPath = "/ws/bus"
	}
	if opts.WithOtherBus == nil {
		opts.WithOtherBus = New()
	}
	if opts.OnUpgradeWs != nil {
		ws.DefaultUpgraderKSMUX.CheckOrigin = opts.OnUpgradeWs
	}
	if opts.WithOtherRouter == nil {
		opts.WithOtherRouter = ksmux.New(ksmux.Config{
			Address: opts.Address,
		})
	} else {
		if opts.WithOtherRouter.Address() != "" {
			opts.Address = opts.WithOtherRouter.Address()
		} else if opts.WithOtherRouter.Config.Domain != "" {
			opts.Address = opts.WithOtherRouter.Config.Domain
		}
	}
	if len(opts.OnServerData) == 0 {
		opts.OnServerData = []func(data any, conn *ws.Conn){}
	}

	server := Server{
		ID:                      opts.ID,
		Address:                 opts.Address,
		Path:                    opts.BusPath,
		Bus:                     opts.WithOtherBus,
		App:                     opts.WithOtherRouter,
		sendToServerConnections: kmap.New[string, *ws.Conn](20),
		onWsClose:               opts.OnWsClose,
		onDataWS:                opts.OnDataWS,
		onServerData:            opts.OnServerData,
		onId:                    opts.OnId,
		beforeUpgradeWs:         opts.OnUpgradeWs,
		idConnRPC:               kmap.New[string, *RPCConn](10),
		rpcMaxQueueSize:         1000,
	}
	if len(opts.BusMidws) > 0 {
		server.busMidws = opts.BusMidws
	}
	if opts.WithRPCAddress != "" {
		err := server.EnableRPC(opts.WithRPCAddress)
		if err != nil {
			lg.Fatal("Failed to enable RPC:", "err", err)
		}
	}
	server.handleWS()
	return &server
}

func (s *Server) OnWsClose(fn func(connID string)) {
	s.onWsClose = fn
}

func (s *Server) OnServerData(fn func(data any, conn *ws.Conn)) {
	s.onServerData = append(s.onServerData, fn)
}

func (s *Server) OnDataWs(fn func(data map[string]any, conn *ws.Conn, originalRequest *http.Request) error) {
	s.onDataWS = fn
}
func (s *Server) OnId(fn func(data map[string]any)) {
	s.onId = fn
}

func (s *Server) WithPprof(path ...string) {
	s.App.WithPprof(path...)
}

func (s *Server) WithMetrics(httpHandler http.Handler, path ...string) {
	s.App.WithMetrics(httpHandler, path...)
}

func (s *Server) Subscribe(topic string, fn func(data map[string]any, unsub Unsub)) (unsub Unsub) {
	return s.Bus.Subscribe(topic, fn, func(data map[string]any) {
		id, okID := data["id"]
		if eventID, ok := data["event_id"]; ok && okID && id == s.ID {
			s.Publish(eventID.(string), map[string]any{
				"ok":   "done",
				"from": s.ID,
			})
		}
		delete(data, "event_id")
	})
}

func (s *Server) Unsubscribe(topic string) {
	s.Bus.Unsubscribe(topic)
}

func (srv *Server) Publish(topic string, data map[string]any) {
	if _, ok := data["from"]; !ok {
		data["from"] = srv.ID
	}
	data["topic"] = topic
	srv.Bus.Publish(topic, data)
}

func (s *Server) PublishToID(id string, data map[string]any) {
	if _, ok := data["from"]; !ok {
		data["from"] = s.ID
	}

	if rpcConn, ok := s.idConnRPC.Get(id); ok {
		msg := map[string]any{
			"to_id": id,
			"from":  data["from"],
		}
		for k, v := range data {
			if k != "to_id" && k != "from" {
				msg[k] = v
			}
		}
		select {
		case rpcConn.msgChan <- msg:
		default:
			<-rpcConn.msgChan
			rpcConn.msgChan <- msg
		}
		return
	}

	s.Bus.PublishToID(id, data)
}

func (s *Server) PublishWaitRecv(topic string, data map[string]any, onRecv func(data map[string]any), onExpire func(eventId string, topic string)) {
	if _, ok := data["from"]; !ok {
		data["from"] = s.ID
	}
	data["topic"] = topic
	eventId := GenerateUUID()
	data["event_id"] = eventId
	done := make(chan struct{})

	subs := s.Subscribe(eventId, func(data map[string]any, unsub Unsub) {
		done <- struct{}{}
		if onRecv != nil {
			onRecv(data)
		}
		unsub.Unsubscribe()
	})
	s.Publish(topic, data)
free:
	for {
		select {
		case <-done:
			break free
		case <-time.After(500 * time.Millisecond):
			if onExpire != nil {
				onExpire(eventId, topic)
			}
			subs.Unsubscribe()
			break free
		}
	}
}

func (s *Server) PublishToIDWaitRecv(id string, data map[string]any, onRecv func(data map[string]any), onExpire func(eventId string, toID string)) {
	if _, ok := data["from"]; !ok {
		data["from"] = s.ID
	}
	eventId := GenerateUUID()
	data["event_id"] = eventId
	done := make(chan struct{})

	subs := s.Subscribe(eventId, func(data map[string]any, unsub Unsub) {
		done <- struct{}{}
		if onRecv != nil {
			onRecv(data)
		}
		unsub.Unsubscribe()
	})
	s.PublishToID(id, data)
free:
	for {
		select {
		case <-done:
			break free
		case <-time.After(500 * time.Millisecond):
			if onExpire != nil {
				onExpire(eventId, id)
			}
			subs.Unsubscribe()
			break free
		}
	}
}

func (s *Server) RemoveTopic(topic string) {
	s.Bus.RemoveTopic(topic)
}

func (s *Server) PublishToServer(addr string, data map[string]any, secure ...bool) error {
	sch := "ws"
	if len(secure) > 0 && secure[0] {
		sch = "wss"
	}
	u := url.URL{Scheme: sch, Host: addr, Path: s.Path}
	conn, ok := s.sendToServerConnections.Get(addr)
	if !ok {
		var err error
		conn, _, err = ws.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			lg.Printfs("rdSendToServer Dial %s error:%v\n", u.String(), err)
			return err
		}
	}
	dd := map[string]any{
		"action":     "server_message",
		"addr":       addr,
		"data":       data,
		"via_server": s.ID,
	}
	if err := conn.WriteJSON(dd); err != nil {
		lg.Printfs("rdSendToServer WriteJSON on %s error:%v\n", u.String(), err)
		return err
	}
	return nil
}

// RUN
func (s *Server) Run() {
	s.App.Run()
}

func (s *Server) RunTLS() {
	s.App.RunTLS()
}

func (s *Server) RunAutoTLS() {
	s.App.RunAutoTLS()
}

func (s *Server) EnableRPC(address string) error {
	// Register types for gob encoding
	gob.Register(map[string]interface{}{})

	s.rpcServer = rpc.NewServer()

	busRPC := &BusRPC{server: s}
	err := s.rpcServer.RegisterName("BusRPC", busRPC)
	if err != nil {
		return err
	}

	s.rpcServer.HandleHTTP(rpc.DefaultRPCPath, rpc.DefaultDebugPath)

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}

	go http.Serve(listener, nil)
	return nil
}

type BusRPC struct {
	server *Server
}

func (b *BusRPC) Ping(req *RPCRequest, resp *RPCResponse) error {
	if _, ok := b.server.idConnRPC.Get(req.From); !ok {
		rpcConn := &RPCConn{
			Id:      req.From,
			msgChan: make(chan map[string]any, b.server.rpcMaxQueueSize),
		}
		b.server.idConnRPC.Set(req.From, rpcConn)
	}
	return nil
}

func (b *BusRPC) Subscribe(req *RPCRequest, resp *RPCResponse) error {
	rpcConn, ok := b.server.idConnRPC.Get(req.From)
	if !ok {
		return fmt.Errorf("client not registered")
	}

	sub := Subscriber{
		bus:   b.server.Bus,
		Id:    req.From,
		Topic: req.Topic,
		Ch:    rpcConn.msgChan,
	}

	if subs, found := b.server.Bus.topicSubscribers.Get(req.Topic); found {
		subs = append(subs, sub)
		b.server.Bus.topicSubscribers.Set(req.Topic, subs)
	} else {
		b.server.Bus.topicSubscribers.Set(req.Topic, []Subscriber{sub})
	}

	return nil
}

func (b *BusRPC) Unsubscribe(req *RPCRequest, resp *RPCResponse) error {
	if subs, ok := b.server.Bus.topicSubscribers.Get(req.Topic); ok {
		for i := range subs {
			if subs[i].Id == req.From {
				subs = append(subs[:i], subs[i+1:]...)
				b.server.Bus.topicSubscribers.Set(req.Topic, subs)
				break
			}
		}
	}
	return nil
}

func (b *BusRPC) Publish(req *RPCRequest, resp *RPCResponse) error {
	req.Data["from"] = req.From
	msg := map[string]any{
		"from":  req.From,
		"topic": req.Topic,
	}
	// Copy all other fields
	for k, v := range req.Data {
		if k != "from" && k != "topic" {
			msg[k] = v
		}
	}

	b.server.Bus.Publish(req.Topic, msg)
	return nil
}

func (b *BusRPC) PublishToID(req *RPCRequest, resp *RPCResponse) error {
	req.Data["from"] = req.From
	msg := map[string]any{
		"to_id": req.Id,
		"from":  req.From,
	}
	for k, v := range req.Data {
		if k != "to_id" && k != "from" {
			msg[k] = v
		}
	}

	if eventID, ok := req.Data["event_id"]; ok {
		msg["event_id"] = eventID
	}

	if req.Id == b.server.ID {
		if b.server.onId != nil {
			b.server.onId(msg)
			if eventID, ok := msg["event_id"]; ok {
				b.server.Bus.Publish(eventID.(string), map[string]any{
					"ok":   "done",
					"from": b.server.ID,
				})
			}
			return nil
		}
	}

	if rpcConn, ok := b.server.idConnRPC.Get(req.Id); ok {
		select {
		case rpcConn.msgChan <- msg:
		default:
			<-rpcConn.msgChan
			rpcConn.msgChan <- msg
		}
		return nil
	}

	b.server.Bus.PublishToID(req.Id, msg)
	return nil
}

func (b *BusRPC) RemoveTopic(req *RPCRequest, resp *RPCResponse) error {
	req.Data["from"] = req.From
	b.server.Bus.RemoveTopic(req.Topic)
	return nil
}

func (b *BusRPC) Poll(req *RPCRequest, resp *RPCResponse) error {
	rpcConn, ok := b.server.idConnRPC.Get(req.From)
	if !ok {
		return fmt.Errorf("client not registered")
	}

	select {
	case msg := <-rpcConn.msgChan:
		resp.Data = msg
		return nil
	default:
		resp.Data = nil
		return nil
	}
}

func (s *Server) SetRPCMaxQueueSize(size int) {
	s.rpcMaxQueueSize = size
}
