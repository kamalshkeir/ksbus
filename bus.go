// Package KSbus is a minimalistic event bus demonstration on how useful is to design event based systems using Golang, allowing you to synchronise your backends and frontends
//
// # What is included in this package ?
//  1. Internal Bus [New]
//  2. Golang Server Bus [NewServer]
//  4. JS Client Bus
//  5. Golang Client Bus
//
// This package use [Kmux] wich is the same router as [Korm]
//
// # KSbus can be used between your go servers, and between your servers and client using the built in JS library
//
// [Korm]: https://github.com/kamalshkeir/korm
//
// [Ksmux]: https://github.com/kamalshkeir/ksmux
package ksbus

import (
	"sync"
	"time"

	"github.com/kamalshkeir/kmap"
	"github.com/kamalshkeir/ksmux/ws"
)

// Bus type handle all subscriptions, websockets and channels
type Bus struct {
	topicSubscribers *kmap.SafeMap[string, []Subscriber]
	allWS            *kmap.SafeMap[*ws.Conn, string]
	idConn           *kmap.SafeMap[string, *ws.Conn]
	mu               sync.RWMutex
}

// New return new Bus
func New() *Bus {
	return &Bus{
		topicSubscribers: kmap.New[string, []Subscriber](),
		allWS:            kmap.New[*ws.Conn, string](),
		idConn:           kmap.New[string, *ws.Conn](),
	}
}

func (b *Bus) Subscribe(topic string, fn func(data map[string]any, unsub Unsub), onData ...func(data map[string]any)) Unsub {
	sub := Subscriber{
		Id:    "INTERNAL",
		Topic: topic,
		Ch:    make(chan map[string]any),
		bus:   b,
	}

	if subs, found := b.topicSubscribers.Get(topic); found {
		subs = append(subs, sub)
		b.topicSubscribers.Set(topic, subs)
	} else {
		b.topicSubscribers.Set(topic, []Subscriber{sub})
	}

	go func() {
		for v := range sub.Ch {
			for _, fnData := range onData {
				fnData(v)
			}
			fn(v, sub)
		}
	}()
	return sub
}

func (b *Bus) Unsubscribe(topic string) {
	if subs, ok := b.topicSubscribers.Get(topic); ok {
		for i, sub := range subs {
			if sub.Id == "INTERNAL" {
				if sub.Ch != nil {
					close(sub.Ch)
				}
				subs = append(subs[:i], subs[i+1:]...)
				b.topicSubscribers.Set(topic, subs)
			}
		}
	}
}

func (b *Bus) Publish(topic string, data map[string]any) {
	if _, ok := data["from"]; !ok {
		data["from"] = "INTERNAL"
	}
	data["topic"] = topic

	if subs, found := b.topicSubscribers.Get(topic); found {
		b.mu.Lock()
		defer b.mu.Unlock()
		for _, s := range subs {
			if s.Ch != nil {
				s.Ch <- data
			} else {
				_ = s.Conn.WriteJSON(data)
			}
		}
	}

}

func (b *Bus) PublishToID(id string, data map[string]any) {
	if _, ok := data["from"]; !ok {
		data["from"] = "INTERNAL"
	}
	data["to_id"] = id

	if conn, ok := b.idConn.Get(id); ok {
		b.mu.Lock()
		defer b.mu.Unlock()
		_ = conn.WriteJSON(data)
	}
}

func (b *Bus) PublishWaitRecv(topic string, data map[string]any, onRecv func(data map[string]any), onExpire func(eventId string, topic string)) error {
	if _, ok := data["from"]; !ok {
		data["from"] = "INTERNAL"
	}
	data["topic"] = topic
	eventId := GenerateUUID()
	data["event_id"] = eventId
	done := make(chan struct{})
	subs := b.Subscribe(eventId, func(data map[string]any, unsub Unsub) {
		done <- struct{}{}
		if onRecv != nil {
			onRecv(data)
		}
		unsub.Unsubscribe()
	})
	b.Publish(topic, data)
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
	return nil
}

func (b *Bus) PublishToIDWaitRecv(id string, data map[string]any, onRecv func(data map[string]any), onExpire func(eventId string, id string)) error {
	if _, ok := data["from"]; !ok {
		data["from"] = "INTERNAL"
	}
	data["id"] = id
	eventId := GenerateUUID()
	data["event_id"] = eventId
	done := make(chan struct{})

	subs := b.Subscribe(eventId, func(data map[string]any, unsub Unsub) {
		done <- struct{}{}
		if onRecv != nil {
			onRecv(data)
		}
		unsub.Unsubscribe()
	})
	b.PublishToID(id, data)
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
	return nil
}

func (b *Bus) RemoveTopic(topic string) {
	go b.topicSubscribers.Delete(topic)
}
