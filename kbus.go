// Package KSbus is a minimalistic event bus demonstration on how useful is to design event based systems using Golang, allowing you to synchronise your backends and frontends
//
// # What is included in this package ?
//  1. Internal Bus [New]
//  2. Golang Server Bus [NewServer]
//  3. Golang Combined Servers Bus [NewCombinedServer]
//  4. JS Client Bus
//  5. Golang Client Bus
//
// This package use [Kmux] wich is the same router as [Kago]
//
// # KSbus can be used between your go servers, and between your servers and client using the built in JS library
//
// [Kago]: https://github.com/kamalshkeir/kago
// [Kmux]: https://github.com/kamalshkeir/kmux
package ksbus

import (
	"sync"

	"github.com/kamalshkeir/kmap"
)

// Bus type handle all subscriptions, websockets and channels
type Bus struct {
	subscribers   *kmap.SafeMap[string, []Channel]
	wsSubscribers *kmap.SafeMap[string, []ClientSubscription]
	channelsName  *kmap.SafeMap[Channel, []string]
	mu            sync.Mutex
}

// New return new Bus
func New() *Bus {
	return &Bus{
		subscribers:   kmap.New[string, []Channel](false),
		wsSubscribers: kmap.New[string, []ClientSubscription](false),
		channelsName:  kmap.New[Channel, []string](false),
	}
}

// Subscribe let you subscribe to a topic and return a unsubscriber channel
func (b *Bus) Subscribe(topic string, fn func(data map[string]any, ch Channel), name ...string) (ch Channel) {
	// add sub
	nn := ""
	if len(name) > 0 && name[0] != "" {
		nn = name[0]
	}
	ch = Channel{
		Id:    GenerateRandomString(5),
		Ch:    make(chan map[string]any),
		Topic: topic,
		Name:  nn,
		Bus:   b,
	}
	if nn != "" {
		b.setName(ch, nn)
	}
	if subs, found := b.subscribers.Get(topic); found {
		subs = append(subs, ch)
		b.subscribers.Set(topic, subs)
	} else {
		subs = append([]Channel{}, ch)
		b.subscribers.Set(topic, subs)
	}

	go func() {
		for v := range ch.Ch {
			fn(v, ch)
		}
	}()
	return ch
}

func (b *Bus) Unsubscribe(topic string, ch Channel) {
	// add sub
	if subs, found := ch.Bus.subscribers.Get(topic); found {
		for i, sub := range subs {
			if sub == ch {
				subs = append(subs[:i], subs[i+1:]...)
				ch.Bus.subscribers.Set(topic, subs)
				close(ch.Ch)
			}
		}
	}
	b.channelsName.Delete(ch)
}

func (b *Bus) UnsubscribeId(topic, id string) {
	// add sub
	if subs, found := b.subscribers.Get(topic); found {
		for i, sub := range subs {
			if sub.Id == id {
				subs = append(subs[:i], subs[i+1:]...)
				b.subscribers.Set(topic, subs)
				close(sub.Ch)
			}
		}
	}
	b.channelsName.Range(func(key Channel, value []string) {
		if key.Id == id {
			go b.channelsName.Delete(key)
		}
	})
}

func (b *Bus) Publish(topic string, data map[string]any) {
	data["topic"] = topic
	if chans, found := b.subscribers.Get(topic); found {
		channels := append([]Channel{}, chans...)
		go func(data map[string]any, subs []Channel) {
			for _, ch := range subs {
				ch.Ch <- data
			}
		}(data, channels)
	}
}

func (b *Bus) RemoveTopic(topic string) {
	go b.subscribers.Delete(topic)
	go b.wsSubscribers.Delete(topic)
	clientSubNames.Range(func(key ClientSubscription, topicss []string) {
		for i, v := range topicss {
			if v == topic {
				topicss = append(topicss[:i], topicss[i+1:]...)
				go clientSubNames.Set(key, topicss)
			}
		}
	})
}

func (b *Bus) SendToNamed(name string, data map[string]any) {
	data["name"] = name
	b.channelsName.Range(func(key Channel, value []string) {
		for _, v := range value {
			if v == name {
				key.Ch <- data
			}
		}
	})
}
