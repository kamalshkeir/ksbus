package ksbus

type Channel struct {
	Id    string
	Topic string
	Name  string
	Ch    chan map[string]any
	Bus   *Bus
}

func (ch Channel) Unsubscribe() Channel {
	if subs, found := ch.Bus.subscribers.Get(ch.Topic); found {
		for i, sub := range subs {
			if sub == ch && (sub.Name == ch.Name || sub.Name == ch.Topic+":"+ch.Name) {
				subs = append(subs[:i], subs[i+1:]...)
			}
		}
		ch.Bus.Unsubscribe(ch)
		ch.Bus.subscribers.Set(ch.Topic, subs)
	}
	return ch
}
