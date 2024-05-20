package ksbus

import (
	"net/http"
	"net/url"
	"time"

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
	onServerData            func(data any, conn *ws.Conn)
	onId                    func(data map[string]any)
	sendToServerConnections *kmap.SafeMap[string, *ws.Conn]
	beforeUpgradeWs         func(r *http.Request) bool
}

type ServerOpts struct {
	ID              string
	Address         string
	Path            string
	BusMidws        []func(ksmux.Handler) ksmux.Handler
	OnWsClose       func(connID string)
	OnDataWS        func(data map[string]any, conn *ws.Conn, originalRequest *http.Request) error
	OnServerData    func(data any, conn *ws.Conn)
	OnId            func(data map[string]any)
	OnUpgradeWs     func(r *http.Request) bool
	WithOtherRouter *ksmux.Router
	WithOtherBus    *Bus
}

func NewDefaultServerOptions() ServerOpts {
	return ServerOpts{
		ID:              GenerateUUID(),
		Address:         ":9313",
		Path:            "/ws/bus",
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
		opts.Address = ":9313"
	}
	if opts.Path == "" {
		opts.Path = "/ws/bus"
	}
	if opts.WithOtherBus == nil {
		opts.WithOtherBus = New()
	}
	if opts.OnUpgradeWs == nil {
		opts.OnUpgradeWs = func(r *http.Request) bool { return true }
	}
	if opts.WithOtherRouter == nil {
		opts.WithOtherRouter = ksmux.New()
	}
	server := Server{
		ID:                      opts.ID,
		Address:                 opts.Address,
		Path:                    opts.Path,
		Bus:                     opts.WithOtherBus,
		App:                     opts.WithOtherRouter,
		sendToServerConnections: kmap.New[string, *ws.Conn](),
		onWsClose:               opts.OnWsClose,
		onDataWS:                opts.OnDataWS,
		onServerData:            opts.OnServerData,
		onId:                    opts.OnId,
		beforeUpgradeWs:         opts.OnUpgradeWs,
	}
	if len(opts.BusMidws) > 0 {
		server.busMidws = opts.BusMidws
	}
	server.handleWS()
	return &server
}

func (s *Server) OnWsClose(fn func(connID string)) {
	s.onWsClose = fn
}

func (s *Server) OnServerData(fn func(data any, conn *ws.Conn)) {
	s.onServerData = fn
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
		if eventID, ok := data["event_id"]; ok {
			s.Publish(eventID.(string), map[string]any{
				"ok":   "done",
				"from": s.ID,
			})
		}
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
	s.App.Run(s.Address)
}

func (s *Server) RunTLS(cert string, certKey string) {
	s.App.RunTLS(s.Address, cert, certKey)
}

func (s *Server) RunAutoTLS(subDomains ...string) {
	s.App.RunAutoTLS(s.Address, subDomains...)
}
