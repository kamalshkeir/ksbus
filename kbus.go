// Package Kbus is a minimalistic event bus demonstration on how useful is to design event based systems using Golang, allowing you to synchronise your backends and frontends
//
// # What is included in this package ?
//  1. Internal Bus [New]
//  2. Golang Server Bus [NewServer]
//  3. Golang Combined Servers Bus [NewCombinedServer]
//  4. JS Client Bus
//  5. Golang Client Bus
//  6. Coming soon (Python Client)
//
// This package use [Kmux] wich is the same router as [Kago]
//
// # Kbus can be used between your go servers, and between your servers and client using the built in JS library
//
// [Kago]: https://github.com/kamalshkeir/kago
// [Kmux]: https://github.com/kamalshkeir/kmux
package kbus

import (
	"sync"

	"github.com/kamalshkeir/kmap"
)

var bus *Bus

func init() {
	bus = &Bus{
		subscribers: kmap.New[string, []Channel](false),
		wsSubscribers: kmap.New[string, []ClientSubscription](false),
	}
}

// Bus type handle all subscriptions, websockets and channels
type Bus struct {
	subscribers  *kmap.SafeMap[string,[]Channel]
	wsSubscribers *kmap.SafeMap[string, []ClientSubscription]
	mu            sync.Mutex
}

// New return new Bus
func New() *Bus {
	if bus != nil {
		return bus
	} else {
		bus = &Bus{
			subscribers: kmap.New[string, []Channel](false),
			wsSubscribers: kmap.New[string, []ClientSubscription](false),
		}
		return bus
	}
}

// Subscribe subscribe to a topic and return a unsubscriber channeln you can also unsubscribe from the handler, useful for SubscribeOnce or N times
func (b *Bus) Subscribe(topic string, fn func(data map[string]any, ch Channel),name ...string) (ch Channel) {
	// add sub
	nn := ""
	if len(name)>0 && name[0] != "" {
		nn=name[0]
	}
	ch = Channel{
		Id: GenerateRandomString(5),
		Ch: make(chan map[string]any),
		Topic: topic,
		Name: nn,
	}
	if nn != "" {
		setName(ch,nn)
	}
	if subs, found := b.subscribers.Get(topic); found {
		subs = append(subs, ch)
		b.subscribers.Set(topic,subs)
	} else {
		subs = append([]Channel{}, ch)
		b.subscribers.Set(topic,subs)
	}

	go func() {
		for v := range ch.Ch {
			b.mu.Lock()
			fn(v,ch)
			b.mu.Unlock()
		}
	}()
	return ch
}

// Unsubscribe unsubscribe from a topic using channel
func (b *Bus) Unsubscribe(topic string,ch Channel) {
	// add sub
	if subs, found := bus.subscribers.Get(topic); found {
		for i,sub := range subs {
			if sub == ch {
				subs = append(subs[:i],subs[i+1:]... )
				bus.subscribers.Set(topic,subs)
				close(ch.Ch)
			}
		}
	} 
	mChannelName.Delete(ch)
}


// UnsubscribeId unsubscribe from a topic using channel id
func (b *Bus) UnsubscribeId(topic,id string) {
	// add sub
	if subs, found := bus.subscribers.Get(topic); found {
		for i,sub := range subs {
			if sub.Id == id {
				subs = append(subs[:i],subs[i+1:]... )
				bus.subscribers.Set(topic,subs)
				close(sub.Ch)
			}
		}
	} 
	mChannelName.Range(func(key Channel, value []string) {
		if key.Id == id {
			mChannelName.Delete(key)
		}
	})
}


// Publish publish data to a topic
func (b *Bus) Publish(topic string, data map[string]any) {
	b.mu.Lock()
	data["topic"]=topic
	b.mu.Unlock()
	if chans, found := b.subscribers.Get(topic); found {
		channels := append([]Channel{}, chans...)
		go func(data map[string]any, subs []Channel) {
			for _, ch := range subs {
				ch.Ch <- data
			}
		}(data, channels)
	}
}


// RemoveTopic remove a topic completely
func (b *Bus) RemoveTopic(topic string) {
	go b.subscribers.Delete(topic)
	go b.wsSubscribers.Delete(topic)
	mWSName.Range(func(key ClientSubscription, topicss []string) {
		for i,v := range topicss {
			if v == topic {
				topicss=append(topicss[:i],topicss[i+1:]... )
				mWSName.Set(key,topicss)
			}
		}
	})
}

// SendTo send to a named topic
func (b *Bus) SendTo(name string, data map[string]any) {
	data["name"]=name
	mChannelName.Range(func(key Channel, value []string) {
		for _,v := range value {
			if v == name {
				key.Ch <- data
			}
		}
	})
}