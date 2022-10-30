package kbus

import (
	"sync"

	"github.com/kamalshkeir/klog"
	"github.com/kamalshkeir/kmux"
)

type Server struct {
	Bus      *Bus
	App      *kmux.Router
	wg		 sync.WaitGroup
}


func NewServer() *Server {
	app := kmux.New()
	server := Server{
		Bus: bus,
		App: app,
	}
	return &server
}

func (s *Server) Subscribe(topic string, fn func(data map[string]any,ch Channel),name ...string) (ch Channel) {
	if DEBUG{
		klog.Printfs("grSubscribing to %s\n",topic)
	}
	return bus.Subscribe(topic, fn,name...)
}

func (s *Server) Unsubscribe(topic string, ch Channel) {
	if DEBUG{
		klog.Printfs("grUnsubscribing from %s\n",topic)
	}
	if ch.Ch != nil {
		bus.Unsubscribe(topic,ch)
	}
}

func (s *Server) Publish(topic string, data map[string]any) {
	if DEBUG{
		klog.Printfs("grPublish: sending %v on %s \n",data,topic)
	}
	go s.publishWS(topic, data)
	go bus.Publish(topic, data)
}

func (s *Server) RemoveTopic(topic string) {
	if DEBUG {
		klog.Printfs("grRemoving topic %s\n",topic)
	}
	bus.RemoveTopic(topic)
}

func (s *Server) SendTo(name string, data map[string]any) {
	if DEBUG{
		klog.Printfs("grSendTo: sending %v on %s \n",data,name)
	}
	data["name"]=name
	s.wg.Add(2)
	go func(name string, data map[string]any) {
		bus.SendTo(name,data)
		s.wg.Done()
	}(name,data)
	
	go func() {
		defer s.wg.Done()
		mWSName.Range(func(sub ClientSubscription, names []string) {
			for _,n := range names {
				if n == name {
					err := sub.Conn.WriteJSON(data)
					if err != nil {
						klog.Printf("rderr:%v\n",err)
					}
				}
			}
		})
	}()
	s.wg.Wait()
}

// RUN
func (s *Server) Run(addr string) {
	LocalAddress=addr
	s.handleWS(addr)
	s.App.Run(addr)
}

func (s *Server) RunTLS(addr string, cert string, certKey string) {
	LocalAddress=addr
	s.handleWS(addr)
	s.App.RunTLS(addr,cert,certKey)
}

func (s *Server) RunAutoTLS(domainName string, subDomains ...string) {
	LocalAddress=domainName
	s.handleWS(domainName)
	s.App.RunAutoTLS(domainName, subDomains...)
}


