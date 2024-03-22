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
	subscribers   *kmap.SafeMap[string, []Channel]
	wsSubscribers *kmap.SafeMap[string, []ClientSubscription]
	allWS         *kmap.SafeMap[*ws.Conn, string]
	mu            sync.Mutex
}

// New return new Bus
func New() *Bus {
	return &Bus{
		subscribers:   kmap.New[string, []Channel](false),
		wsSubscribers: kmap.New[string, []ClientSubscription](false),
		allWS:         kmap.New[*ws.Conn, string](false),
	}
}

// Subscribe let you subscribe to a topic and return a unsubscriber channel
func (b *Bus) Subscribe(topic string, fn func(data map[string]any, ch Channel), onData ...func(data map[string]any)) (ch Channel) {
	// add sub
	ch = Channel{
		Ch:    make(chan map[string]any),
		Topic: topic,
		bus:   b,
	}
	if subs, found := b.subscribers.Get(topic); found {
		subs = append(subs, ch)
		b.subscribers.Set(topic, subs)
	} else {
		b.subscribers.Set(topic, []Channel{ch})
	}

	go func() {
		for v := range ch.Ch {
			if len(onData) > 0 && onData[0] != nil {
				onData[0](v)
			}
			fn(v, ch)
		}
	}()
	return ch
}

func (b *Bus) Unsubscribe(ch Channel) {
	// add sub
	if subs, found := ch.bus.subscribers.Get(ch.Topic); found {
		for i, sub := range subs {
			if sub == ch {
				subs = append(subs[:i], subs[i+1:]...)
			}
		}
		b.mu.Lock()
		b.subscribers.Set(ch.Topic, subs)
		b.mu.Unlock()
	}
	close(ch.Ch)
}

func (b *Bus) Publish(topic string, data map[string]any) error {
	if _, ok := data["from"]; !ok {
		data["from"] = "INTERNAL"
	}
	data["topic"] = topic
	if chans, found := b.subscribers.Get(topic); found {
		channels := append([]Channel{}, chans...)
		go func() {
			for _, ch := range channels {
				ch.Ch <- data
			}
		}()
	} else {
		return ErrNotFound
	}
	return nil
}

func (b *Bus) PublishToID(id string, data map[string]any) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := data["from"]; !ok {
		data["from"] = "INTERNAL"
	}
	data["to_id"] = id
	found := false
	b.allWS.Range(func(key *ws.Conn, value string) {
		if value == id {
			found = true
			_ = key.WriteJSON(data)
			return
		}
	})
	if !found {
		return ErrNotFound
	}
	return nil
}

func (b *Bus) PublishWaitRecv(topic string, data map[string]any, onRecv func(data map[string]any), onExpire func(eventId string, topic string)) error {
	if _, ok := data["from"]; !ok {
		data["from"] = "INTERNAL"
	}
	data["topic"] = topic
	eventId := GenerateUUID()
	data["event_id"] = eventId
	done := make(chan struct{})

	subs := b.Subscribe(eventId, func(data map[string]any, ch Channel) {
		done <- struct{}{}
		if onRecv != nil {
			onRecv(data)
		}
		ch.Unsubscribe()
	})
	err := b.Publish(topic, data)
	if err != nil {
		if onExpire != nil {
			onExpire(eventId, topic)
		}
		subs.Unsubscribe()
		return err
	}
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

	subs := b.Subscribe(eventId, func(data map[string]any, ch Channel) {
		done <- struct{}{}
		if onRecv != nil {
			onRecv(data)
		}
		ch.Unsubscribe()
	})
	err := b.PublishToID(id, data)
	if err != nil {
		if onExpire != nil {
			onExpire(eventId, id)
		}
		subs.Unsubscribe()
		return err
	}
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
	go b.subscribers.Delete(topic)
	go b.wsSubscribers.Delete(topic)
}
