package ksbus

type Channel struct {
	Id    string
	Topic string
	Ch    chan map[string]any
	bus   *Bus
}

func (ch Channel) Unsubscribe() {
	if subs, found := ch.bus.subscribers.Get(ch.Topic); found {
		for i, sub := range subs {
			if sub == ch && sub.Topic == ch.Topic && sub.Id == ch.Id {
				subs = append(subs[:i], subs[i+1:]...)
			}
		}
		ch.bus.Unsubscribe(ch)
		ch.bus.subscribers.Set(ch.Topic, subs)
	}
}
