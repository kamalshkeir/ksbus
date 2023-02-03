package ksbus

import (
	"fmt"
	"time"

	"github.com/kamalshkeir/klog"
	"github.com/kamalshkeir/kmux"
	"github.com/kamalshkeir/kmux/ws"
)

var (
	keepServing = false
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

func (s *Server) addWS(id, topic string, conn *ws.Conn) {
	wsSubscribers := s.Bus.wsSubscribers
	new := ClientSubscription{
		Id:   GenerateRandomString(5),
		Conn: conn,
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

func addNamedWS(topic, name, id string, conn *ws.Conn) {
	clientSub := ClientSubscription{
		Id:    id,
		Name:  name,
		Topic: topic,
		Conn:  conn,
	}
	if v, ok := clientSubNames.Get(clientSub); ok {
		v = append(v, topic+":"+name)
		clientSubNames.Set(ClientSubscription{Conn: conn, Id: id, Topic: topic, Name: name}, v)
	} else {
		clientSubNames.Set(ClientSubscription{Conn: conn, Id: id, Topic: topic, Name: name}, []string{topic + ":" + name})
	}
}

func (s *Server) removeWS(wsConn *ws.Conn) {
	go func() {
		clientSubNames.Range(func(key ClientSubscription, value []string) {
			if key.Conn == wsConn {
				go clientSubNames.Delete(key)
			}
		})
		s.Bus.wsSubscribers.Range(func(key string, value []ClientSubscription) {
			for i, sub := range value {
				if sub.Conn == wsConn {
					value = append(value[:i], value[i+1:]...)
					go s.Bus.wsSubscribers.Set(key, value)
					if len(value) == 0 {
						s.localTopics.Delete(key)
						if keepServing {
							s.subscribedServersClients.Range(func(addr string, value *Client) {
								go s.sendData(addr, map[string]any{
									"action":          "remove_node_topic",
									"addr":            LocalAddress,
									"topic_to_delete": key,
								})
							})
						}
					}
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
		s.Bus.wsSubscribers.Range(func(key string, value []ClientSubscription) {
			for i, sub := range value {
				if sub.Conn == wsConn {
					value = append(value[:i], value[i+1:]...)
					go s.Bus.wsSubscribers.Set(key, value)
				}
			}
		})
		clientSubNames.Range(func(key ClientSubscription, value []string) {
			if key.Conn == wsConn {
				go clientSubNames.Delete(key)
			}
		})
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
	clientSubNames.Range(func(key ClientSubscription, value []string) {
		for _, v := range value {
			res[v] = struct{}{}
		}
	})
	n := []string{}
	for k := range res {
		n = append(n, k)
	}
	return n
}

func (server *Server) handleWS(addr string) {
	ws.FuncBeforeUpgradeWS = BeforeUpgradeWS
	server.App.WS(ServerPath, func(c *kmux.WsContext) {
		for {
			m, err := c.ReceiveJson()
			if err != nil || !BeforeDataWS(m, c.Ws, c.Request) {
				if err != nil && DEBUG {
					klog.Printf("rd%v\n", err)
				}
				server.removeWS(c.Ws)
				break
			}

			if keepServing {
				send := true
				if publ, ok := m["from_publisher"]; ok && publ.(string) == "master" {
					send = false
				}
				if send {
					server.subscribedServersClients.Range(func(key string, value *Client) {
						m["from_publisher"] = LocalAddress
						go server.sendData(key, m)
					})
				}
			}

			if DEBUG {
				klog.Printfs("--------------------------------\n")
				klog.Printfs("yl%v \n", m)
			}
			server.handleActions(m, c)
		}
	})
}

func (server *Server) handleActions(m map[string]any, c *kmux.WsContext) {
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
						c.Json(map[string]any{
							"error": "topic missing",
						})
					}
				case map[string]any:
					if topic, ok := m["topic"]; ok {
						server.Publish(topic.(string), v)
					} else {
						c.Json(map[string]any{
							"error": "topic missing",
						})
					}
				default:
					c.Json(map[string]any{
						"error": "type not handled, only json or object stringified",
					})
				}
			}

		case "sub", "subscribe":
			if topic, ok := m["topic"]; ok {
				if id, ok := m["id"]; ok {
					server.Bus.mu.Lock()
					server.addWS(id.(string), topic.(string), c.Ws)
					server.Bus.mu.Unlock()
					server.localTopics.Set(topic.(string), true)
				} else {
					fmt.Println("id not found, will not be added:", m)
				}
				if v, ok := m["name"]; ok {
					if id, ok := m["id"]; ok {
						addNamedWS(topic.(string), v.(string), id.(string), c.Ws)
						server.localTopics.Set(topic.(string)+":"+v.(string), true)
					} else {
						fmt.Println("id not found, will not be added:", m)
					}
				}
				if keepServing {
					go func() {
						server.subscribedServersClients.Range(func(key string, value *Client) {
							go server.sendData(key, map[string]any{
								"from_publisher": LocalAddress,
								"action":         "topics",
								"addr":           LocalAddress,
								"topics":         server.localTopics.Keys(),
							})
						})
					}()
				}
			} else {
				c.Json(map[string]any{
					"error": "topic missing",
				})
			}

		case "unsub", "unsubscribe":
			if topic, ok := m["topic"]; ok {
				if id, ok := m["id"]; ok {
					server.unsubscribeWS(id.(string), topic.(string), c.Ws)
				} else {
					fmt.Println("id not found, will not be removed:", m)
				}
				if nn, ok := m["name"]; ok {
					clientSubNames.Range(func(key ClientSubscription, value []string) {
						for i, v := range value {
							if id, ok := m["id"]; ok {
								if (topic.(string) == nn || topic.(string)+":"+v == nn || v == nn) && key.Id == id {
									value = append(value[:i], value[i+1:]...)
									if len(value) == 0 {
										go server.localTopics.Delete(topic.(string) + ":" + v)
										go server.localTopics.Delete(v)
									}
									go clientSubNames.Set(key, value)
								}
							} else {
								fmt.Println("id not found, will not be removed:", m)
							}
						}
					})
				}
				if keepServing {
					go func() {
						server.subscribedServersClients.Range(func(key string, value *Client) {
							go server.sendData(key, map[string]any{
								"from_publisher": LocalAddress,
								"action":         "topics",
								"addr":           LocalAddress,
								"topics":         server.localTopics.Keys(),
							})
						})
					}()
				}
			}
		case "remove_topic", "removeTopic":
			if topic, ok := m["topic"]; ok {
				server.RemoveTopic(topic.(string))
				if keepServing {
					go func() {
						server.subscribedServersClients.Range(func(key string, value *Client) {
							go server.sendData(key, map[string]any{
								"from_publisher": LocalAddress,
								"action":         "topics",
								"addr":           LocalAddress,
								"topics":         server.localTopics.Keys(),
							})
						})
					}()
				}
			} else {
				c.Json(map[string]any{
					"error": "topic missing",
				})
			}

		case "send", "sendTo":
			if data, ok := m["data"]; ok {
				topic := ""
				if top, ok := m["topic"]; ok && top != "" {
					topic = top.(string)
				}
				switch v := data.(type) {
				case string:
					mm := map[string]any{
						"data": v,
					}
					if name, ok := m["name"]; ok {
						if nn, ok := name.(string); ok {
							if topic != "" {
								server.SendToNamed(topic+":"+nn, mm)
							} else {
								server.SendToNamed(nn, mm)
							}
						}
					} else {
						c.Json(map[string]any{
							"error": "name missing",
						})
					}
				case map[string]any:
					if name, ok := m["name"]; ok {
						if topic != "" {
							server.SendToNamed(topic+":"+name.(string), v)
						} else {
							server.SendToNamed(name.(string), v)
						}
					} else {
						c.Json(map[string]any{
							"error": "name missing",
						})
					}
				default:
					c.Json(map[string]any{
						"error": "type not handled, only json or object stringified",
					})
				}
			}
		case "server_sub":
			if sc, ok := m["secure"]; ok {
				if v, ok := sc.(bool); ok {
					keepServing = true
					client := NewClient()
					err := client.Connect(m["addr"].(string), v)
					klog.CheckError(err)
					server.subscribedServersClients.Set(m["addr"].(string), client)
					topics := server.AllTopics()
					for _, t := range topics {
						server.localTopics.Set(t, true)
					}
					client.sendDataToServer(map[string]any{
						"action": "topics",
						"addr":   LocalAddress,
						"topics": topics,
					})
				} else {
					klog.Printf("rd secure not bool %T\n", v)
				}
			}
		case "server_message", "serverMessage":
			if data, ok := m["data"]; ok {
				if addr, ok := m["addr"]; ok && addr.(string) == LocalAddress {
					BeforeServersData(data, c.Ws)
				}
			}
		case "ping":
			_ = c.Json(map[string]any{
				"data": "pong",
			})
		default:
			_ = c.Json(map[string]any{
				"error": "action " + action.(string) + " not handled",
			})
		}
	}
}

func (b *Bus) setName(ch Channel, name string) {
	ch.Name = name
	if ch.Topic != "" && ch.Name != "" {
		ch.Name = ch.Topic + ":" + ch.Name
	}
	if ch.Id == "" {
		ch.Id = GenerateRandomString(5)
	}
	if v, ok := b.channelsName.Get(ch); ok {
		if !kmux.SliceContains(v, ch.Name) {
			v = append(v, ch.Name)
			b.channelsName.Set(ch, v)
		}
	} else {
		b.channelsName.Set(ch, []string{ch.Name})
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
				fmt.Println("stoping retry after", maxRetry, "times")
				break
			}
		} else {
			err = function()
		}
	}
}
