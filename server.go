package ksbus

import (
	"net/http"
	"net/url"
	"time"

	"github.com/kamalshkeir/klog"
	"github.com/kamalshkeir/kmap"
	"github.com/kamalshkeir/ksmux"
	"github.com/kamalshkeir/ksmux/ws"
)

type Server struct {
	ID                      string
	Bus                     *Bus
	App                     *ksmux.Router
	onData                  func(data map[string]any)
	sendToServerConnections *kmap.SafeMap[string, *ws.Conn]
}

func NewServer(bus ...*Bus) *Server {
	var b *Bus
	if len(bus) > 0 {
		b = bus[0]
	} else {
		b = New()
	}
	app := ksmux.New()
	server := Server{
		ID:                      GenerateUUID(),
		Bus:                     b,
		App:                     app,
		sendToServerConnections: kmap.New[string, *ws.Conn](false),
	}
	return &server
}

func (s *Server) OnData(fn func(data map[string]any)) {
	s.onData = fn
	s.Subscribe("idsssss", func(data map[string]any, _ Channel) {
		s.onData(data)
	})
}

func (s *Server) WithPprof(path ...string) {
	s.App.WithPprof(path...)
}

func (s *Server) WithMetrics(httpHandler http.Handler, path ...string) {
	s.App.WithMetrics(httpHandler, path...)
}

func (s *Server) Subscribe(topic string, fn func(data map[string]any, ch Channel)) (ch Channel) {
	if DEBUG {
		klog.Printfs("grSubscribing to topic %s\n", topic)
	}
	return s.Bus.Subscribe(topic, fn)
}

func (s *Server) Unsubscribe(ch Channel) {
	if ch.Ch != nil {
		s.Bus.Unsubscribe(ch)
	}
}

func (s *Server) Publish(topic string, data map[string]any, from ...string) {
	if len(from) > 0 {
		if _, ok := data["from"]; !ok {
			data["from"] = from[0]
		}
	}
	s.Bus.Publish(topic, data)
	s.publishWS(topic, data)
}

func (s *Server) PublishToID(id string, data map[string]any, from ...string) {
	if len(from) > 0 {
		if _, ok := data["from"]; !ok {
			data["from"] = from[0]
		}
	}
	if id == s.ID {
		s.onData(data)
		return
	}
	s.Bus.PublishToID(id, data)
	s.publishWSToID(id, data)
}

func (s *Server) PublishWaitRecv(topic string, data map[string]any, onRecv func(data map[string]any, ch Channel), from ...string) {
	data["topic"] = topic
	eventId := GenerateRandomString(12)
	data["event_id"] = eventId
	if len(from) > 0 {
		if _, ok := data["from"]; !ok {
			data["from"] = from[0]
		}
	}
	done := make(chan struct{})
	s.Publish(topic, data)
	s.Subscribe(eventId, func(data map[string]any, ch Channel) {
		done <- struct{}{}
		onRecv(data, ch)
	})
free:
	for {
		select {
		case <-done:
			break free
		case <-time.After(500 * time.Millisecond):
			break free
		}
	}
}

func (s *Server) RemoveTopic(topic string) {
	if DEBUG {
		klog.Printfs("grRemoving topic %s\n", topic)
	}
	s.Bus.RemoveTopic(topic)
}

func (s *Server) SendToServer(addr string, data map[string]any, secure ...bool) {
	if DEBUG {
		klog.Printfs("grSendToServer: sending %v on %s \n", data, addr)
	}

	sch := "ws"
	if len(secure) > 0 && secure[0] {
		sch = "wss"
	}
	u := url.URL{Scheme: sch, Host: addr, Path: ServerPath}
	conn, ok := s.sendToServerConnections.Get(addr)
	if !ok {
		var err error
		conn, _, err = ws.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			klog.Printfs("rdSendToServer Dial %s error:%v\n", u.String(), err)
			return
		}
	}
	data = map[string]any{
		"action": "server_message",
		"addr":   addr,
		"data":   data,
	}
	if err := conn.WriteJSON(data); err != nil {
		klog.Printfs("rdSendToServer WriteJSON on %s error:%v\n", u.String(), err)
		return
	}
}

// RUN
func (s *Server) Run(addr string) {
	LocalAddress = addr
	s.handleWS()
	s.App.Run(addr)
}

func (s *Server) RunTLS(addr string, cert string, certKey string) {
	LocalAddress = addr
	s.handleWS()
	s.App.RunTLS(addr, cert, certKey)
}

func (s *Server) RunAutoTLS(domainName string, subDomains ...string) {
	LocalAddress = domainName
	s.handleWS()
	s.App.RunAutoTLS(domainName, subDomains...)
}
