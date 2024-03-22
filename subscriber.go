package ksbus

import "github.com/kamalshkeir/ksmux/ws"

type Unsub interface {
	Unsubscribe()
}

type Subscriber struct {
	bus   *Bus
	Id    string
	Topic string
	Ch    chan map[string]any
	Conn  *ws.Conn
}

func (subs Subscriber) Unsubscribe() {
	if allSubs, ok := subs.bus.topicSubscribers.Get(subs.Topic); ok {
		for i := len(allSubs) - 1; i >= 0; i-- {
			if (subs.Conn != nil && subs.Conn == allSubs[i].Conn) || (subs.Ch != nil && subs.Ch == allSubs[i].Ch) {
				allSubs = append(allSubs[:i], allSubs[i+1:]...)
				break
			}
		}
		subs.bus.topicSubscribers.Set(subs.Topic, allSubs)
	}
}
