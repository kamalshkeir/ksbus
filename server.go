package ksbus

import (
	"net/url"
	"sync"

	"github.com/kamalshkeir/klog"
	"github.com/kamalshkeir/kmap"
	"github.com/kamalshkeir/kmux"
	"github.com/kamalshkeir/kmux/ws"
)

type Server struct {
	Bus                      *Bus
	App                      *kmux.Router
	sendToServerConnections  *kmap.SafeMap[string, *ws.Conn]
	subscribedServersClients *kmap.SafeMap[string, *Client]
	localTopics              *kmap.SafeMap[string, bool]
	mu                       sync.Mutex
}

func NewServer(bus ...*Bus) *Server {
	var b *Bus
	if len(bus) > 0 {
		b = bus[0]
	} else {
		b = New()
	}
	app := kmux.New()
	server := Server{
		Bus:                      b,
		App:                      app,
		sendToServerConnections:  kmap.New[string, *ws.Conn](false),
		subscribedServersClients: kmap.New[string, *Client](false),
		localTopics:              kmap.New[string, bool](false),
	}
	return &server
}

func (s *Server) JoinCombinedServer(combinedAddr string, secure bool) error {
	keepServing = true
	client := NewClient()
	err := client.Connect(combinedAddr, secure)
	if err != nil {
		return err
	}
	s.subscribedServersClients.Set(combinedAddr, client)
	s.sendData(combinedAddr, map[string]any{
		"action": "new_node",
		"addr":   LocalAddress,
		"secure": secure,
	})
	return nil
}

func (s *Server) Subscribe(topic string, fn func(data map[string]any, ch Channel), name ...string) (ch Channel) {
	if DEBUG {
		klog.Printfs("grSubscribing to topic %s\n", topic)
	}
	return s.Bus.Subscribe(topic, fn, name...)
}

func (s *Server) Unsubscribe(topic string, ch Channel) {
	if ch.Ch != nil {
		s.Bus.Unsubscribe(topic, ch)
	}
}

func (s *Server) Publish(topic string, data map[string]any) {
	go func() {
		s.publishWS(topic, data)
		s.Bus.Publish(topic, data)
	}()
}

func (s *Server) RemoveTopic(topic string) {
	if DEBUG {
		klog.Printfs("grRemoving topic %s\n", topic)
	}
	s.Bus.RemoveTopic(topic)
}

func (s *Server) SendTo(name string, data map[string]any) {
	if DEBUG {
		klog.Printfs("grSendTo: sending %v on %s \n", data, name)
	}
	data["name"] = name
	go func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.Bus.SendTo(name, data)
	}()
	clientSubNames.Range(func(sub ClientSubscription, names []string) {
		for _, n := range names {
			if n == name {
				s.mu.Lock()
				defer s.mu.Unlock()
				err := sub.Conn.WriteJSON(data)
				if err != nil {
					klog.Printf("rderr:%v\n", err)
				}
			}
		}
	})
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

func (s *Server) sendData(addr string, data map[string]any, secure ...bool) *ws.Conn {

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
			klog.Printfs("rdsendData Dial %s error:%v\n", u.String(), err)
			return nil
		}
	}
	if err := conn.WriteJSON(data); err != nil {
		klog.Printfs("rdsendData WriteJSON on %s error:%v\n", u.String(), err)
		return nil
	}
	return conn
}

// RUN
func (s *Server) Run(addr string) {
	LocalAddress = addr
	s.handleWS(addr)
	s.App.Run(addr)
}

func (s *Server) RunTLS(addr string, cert string, certKey string) {
	LocalAddress = addr
	s.handleWS(addr)
	s.App.RunTLS(addr, cert, certKey)
}

func (s *Server) RunAutoTLS(domainName string, subDomains ...string) {
	LocalAddress = domainName
	s.handleWS(domainName)
	s.App.RunAutoTLS(domainName, subDomains...)
}
