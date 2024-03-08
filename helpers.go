package ksbus

import (
	"time"

	"github.com/kamalshkeir/klog"
	"github.com/kamalshkeir/ksmux"
	"github.com/kamalshkeir/ksmux/ws"
)

func (s *Server) publishWS(topic string, data map[string]any) {
	s.Bus.mu.Lock()
	defer s.Bus.mu.Unlock()
	data["topic"] = topic
	if subscriptions, ok := s.Bus.wsSubscribers.Get(topic); ok {
		for _, sub := range subscriptions {
			_ = sub.Conn.WriteJSON(data)
		}
	}
}

func (s *Server) publishWSToID(id string, data map[string]any) {
	s.Bus.mu.Lock()
	defer s.Bus.mu.Unlock()
	data["id"] = id
	s.Bus.wsSubscribers.Range(func(_ string, value []ClientSubscription) {
		for i := range value {
			if value[i].Id == id {
				_ = value[i].Conn.WriteJSON(data)
				return
			}
		}
	})
}

func (s *Server) addWS(id, topic string, conn *ws.Conn) {
	wsSubscribers := s.Bus.wsSubscribers
	new := ClientSubscription{
		Conn:  conn,
		Id:    id,
		Topic: topic,
	}
	if id == "" {
		GenerateRandomString(5)
	}

	clients, ok := wsSubscribers.Get(topic)
	if ok {
		if len(clients) == 0 {
			clients = []ClientSubscription{new}
			wsSubscribers.Set(topic, clients)
		} else {
			found := false
			for _, c := range clients {
				if c.Conn == conn {
					found = true
				}
			}
			if !found {
				clients = append(clients, new)
				wsSubscribers.Set(topic, clients)
			}
		}
	} else {
		clients = []ClientSubscription{new}
		wsSubscribers.Set(topic, clients)
	}
}

func (s *Server) removeWSFromAllTopics(wsConn *ws.Conn) {
	go func() {
		s.Bus.wsSubscribers.Range(func(key string, value []ClientSubscription) {
			for i, sub := range value {
				if sub.Conn == wsConn {
					value = append(value[:i], value[i+1:]...)
					go s.Bus.wsSubscribers.Set(key, value)
				}
			}
		})

		s.sendToServerConnections.Range(func(key string, value *ws.Conn) {
			if value == wsConn {
				go s.sendToServerConnections.Delete(key)
			}
		})
	}()
}

func (s *Server) unsubscribeWS(id, topic string, wsConn *ws.Conn) {
	go func() {
		if clients, ok := s.Bus.wsSubscribers.Get(topic); ok {
			for i, sub := range clients {
				if sub.Conn == wsConn && sub.Id == id {
					clients = append(clients[:i], clients[i+1:]...)
					go s.Bus.wsSubscribers.Set(topic, clients)
				}
			}
		}
	}()
}

func (server *Server) AllTopics() []string {
	res := make(map[string]struct{})
	server.Bus.subscribers.Range(func(key string, value []Channel) {
		res[key] = struct{}{}
	})
	server.Bus.wsSubscribers.Range(func(key string, value []ClientSubscription) {
		res[key] = struct{}{}
	})
	n := []string{}
	for k := range res {
		n = append(n, k)
	}
	return n
}

func (server *Server) GetSubscribers(topic string) ([]Channel, []ClientSubscription) {
	var channelsSubs []Channel
	var wsSubs []ClientSubscription
	if ss, ok := server.Bus.subscribers.Get(topic); ok {
		channelsSubs = ss
	}
	if ss, ok := server.Bus.wsSubscribers.Get(topic); ok {
		wsSubs = ss
	}
	return channelsSubs, wsSubs
}

func (server *Server) handleWS() {
	ws.FuncBeforeUpgradeWS = OnUpgradeWS
	server.App.Get(ServerPath, func(c *ksmux.Context) {
		conn, err := ksmux.UpgradeConnection(c.ResponseWriter, c.Request, nil)
		if klog.CheckError(err) {
			return
		}
		for {
			var m map[string]any
			err := conn.ReadJSON(&m)
			if err != nil || !OnDataWS(m, conn, c.Request) {
				if err != nil && DEBUG {
					klog.Printf("rd%v\n", err)
				}
				server.removeWSFromAllTopics(conn)
				break
			}
			if DEBUG {
				klog.Printfs("--------------------------------\n")
				klog.Printfs("yl%v \n", m)
			}
			server.handleActions(m, conn)
		}
	})
}

func (server *Server) handleActions(m map[string]any, conn *ws.Conn) {
	if action, ok := m["action"]; ok {
		switch action {
		case "pub", "publish":
			if data, ok := m["data"]; ok {
				switch v := data.(type) {
				case string:
					mm := map[string]any{
						"data": v,
					}
					if topic, ok := m["topic"]; ok {
						mm["topic"] = topic.(string)
						server.Publish(topic.(string), mm)
					} else {
						_ = conn.WriteJSON(map[string]any{
							"error": "topic missing",
						})
					}
				case map[string]any:
					if topic, ok := m["topic"]; ok {
						server.Publish(topic.(string), v)
					} else {
						_ = conn.WriteJSON(map[string]any{
							"error": "topic missing",
						})
					}
				default:
					_ = conn.WriteJSON(map[string]any{
						"error": "type not handled, only json or object stringified",
					})
				}
			}

		case "sub", "subscribe":
			if topic, ok := m["topic"]; ok {
				if id, ok := m["id"]; ok {
					server.Bus.mu.Lock()
					server.addWS(id.(string), topic.(string), conn)
					server.Bus.mu.Unlock()
				} else {
					klog.Printf("rdid not found\n")
				}
			} else {
				_ = conn.WriteJSON(map[string]any{
					"error": "topic missing",
				})
			}

		case "unsub", "unsubscribe":
			if topic, ok := m["topic"]; ok {
				if id, ok := m["id"]; ok {
					server.unsubscribeWS(id.(string), topic.(string), conn)
				} else {
					klog.Printf("rdid not found, will not be removed:", m)
				}
			}
		case "remove_topic", "removeTopic":
			if topic, ok := m["topic"]; ok {
				server.RemoveTopic(topic.(string))
			} else {
				_ = conn.WriteJSON(map[string]any{
					"error": "topic missing",
				})
			}
		case "server_message", "serverMessage":
			if data, ok := m["data"]; ok {
				if addr, ok := m["addr"]; ok && addr.(string) == LocalAddress {
					OnServersData(data, conn)
				}
			}
		case "pub_id":
			if data, ok := m["data"]; ok {
				switch v := data.(type) {
				case string:
					mm := map[string]any{
						"data": v,
					}
					if id, ok := m["id"]; ok {
						mm["id"] = id.(string)
						server.PublishToID(id.(string), mm)
					} else {
						_ = conn.WriteJSON(map[string]any{
							"error": "id missing",
						})
					}
				case map[string]any:
					if id, ok := m["id"]; ok {
						if from, ok := m["from"]; ok {
							server.PublishToID(id.(string), v, from.(string))
						} else {
							server.PublishToID(id.(string), v)
						}
					} else {
						_ = conn.WriteJSON(map[string]any{
							"error": "id missing",
						})
					}
				default:
					_ = conn.WriteJSON(map[string]any{
						"error": "type not handled, only json",
					})
				}
			}
		case "ping":
			_ = conn.WriteJSON(map[string]any{
				"data": "pong",
			})
		default:
			_ = conn.WriteJSON(map[string]any{
				"error": "action " + action.(string) + " not handled",
			})
		}
	}
}

// Cronjob like
func RunEvery(t time.Duration, fn func() bool) {
	fn()
	c := time.NewTicker(t)
	for range c.C {
		if fn() {
			break
		}
	}
}

func RetryEvery(t time.Duration, function func() error, maxRetry ...int) {
	i := 0
	err := function()
	for err != nil {
		time.Sleep(t)
		i++
		if len(maxRetry) > 0 {
			if i < maxRetry[0] {
				err = function()
			} else {
				klog.Printf("stoping retry after %v times\n")
				break
			}
		} else {
			err = function()
		}
	}
}
