package ksbus

type Channel struct {
	Id    string
	Ch    chan map[string]any
	Topic string
	Name  string
	Bus   *Bus
}

func (ch Channel) Unsubscribe() Channel {
	if ch.Topic != "" && ch.Name != "" {
		ch.Name = ch.Topic + ":" + ch.Name
	}
	if subs, found := ch.Bus.subscribers.Get(ch.Topic); found {
		for i, sub := range subs {
			if sub == ch && (sub.Name == ch.Name || sub.Name == ch.Topic+":"+ch.Name) {
				subs = append(subs[:i], subs[i+1:]...)
				ch.Bus.subscribers.Set(ch.Topic, subs)
			}
		}
	}
	return ch
}
