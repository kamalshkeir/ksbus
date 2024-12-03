package ksbus

import (
	"strings"
	"time"

	"github.com/kamalshkeir/ksmux"
	"github.com/kamalshkeir/ksmux/ws"
	"github.com/kamalshkeir/lg"
)

func (s *Server) subscribeWS(id, topic string, conn *ws.Conn) {
	if id == "" {
		GenerateRandomString(5)
	}
	if subs, ok := s.Bus.topicSubscribers.Get(topic); ok {
		subs = append(subs, Subscriber{
			bus:   s.Bus,
			Id:    id,
			Topic: topic,
			Conn:  conn,
		})
		s.Bus.topicSubscribers.Set(topic, subs)
	} else {
		_ = s.Bus.topicSubscribers.Set(topic, []Subscriber{{
			bus:   s.Bus,
			Id:    id,
			Topic: topic,
			Conn:  conn,
		}})
	}
}

func (s *Server) unsubscribeWS(topic string, wsConn *ws.Conn) {
	if clients, ok := s.Bus.topicSubscribers.Get(topic); ok {
		for i, s := range clients {
			if s.Conn == wsConn {
				clients = append(clients[:i], clients[i+1:]...)
				s.bus.topicSubscribers.Set(topic, clients)
				return
			}
		}
	}
}

func (s *Server) removeWSFromAllTopics(wsConn *ws.Conn) {
	runned := false
	s.Bus.topicSubscribers.Range(func(key string, value []Subscriber) bool {
		for i, v := range value {
			if v.Conn == wsConn {
				value = append(value[:i], value[i+1:]...)
				go s.Bus.topicSubscribers.Set(key, value)
				if s.onWsClose != nil && !runned {
					runned = true
					s.onWsClose(v.Id)
				}
				break
			}
		}
		return false
	})
	go s.Bus.allWS.Delete(wsConn)
	s.Bus.idConn.Range(func(key string, value *ws.Conn) bool {
		if value == wsConn {
			go s.Bus.idConn.Delete(key)
			if s.onWsClose != nil && !runned {
				runned = true
				s.onWsClose(key)
			}
		}
		return false
	})
}

func (server *Server) AllTopics() []string {
	return server.Bus.topicSubscribers.Keys()
}

func (s *Server) GetSubscribers(topic string) []Subscriber {
	if subs, ok := s.Bus.topicSubscribers.Get(topic); ok {
		return subs
	}
	return nil
}

func (server *Server) handleWS() {
	ws.FuncBeforeUpgradeWS = server.beforeUpgradeWs
	handler := handlerBusWs(server)
	if len(server.busMidws) > 0 {
		for _, h := range server.busMidws {
			handler = h(handler)
		}
	}
	server.App.Get(server.Path, handler)
}

func handlerBusWs(server *Server) ksmux.Handler {
	return func(c *ksmux.Context) {
		conn, err := ksmux.UpgradeConnection(c.ResponseWriter, c.Request, nil)
		if lg.CheckError(err) {
			return
		}
		for {
			var m map[string]any
			err := conn.ReadJSON(&m)
			if err != nil {
				lg.DebugC(err.Error())
				server.removeWSFromAllTopics(conn)
				break
			}
			if server.onDataWS != nil {
				if err := server.onDataWS(m, conn, c.Request); err != nil {
					_ = conn.WriteJSON(map[string]any{
						"error": err.Error(),
					})
					continue
				}
			}
			server.handleActions(m, conn)
		}
	}
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
						if from, ok := m["from"]; ok {
							v["from"] = from
						} else if cc, ok := server.Bus.allWS.Get(conn); ok {
							v["from"] = cc
						}

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
				if from, ok := m["from"]; ok {
					server.subscribeWS(from.(string), topic.(string), conn)
				} else if cc, ok := server.Bus.allWS.Get(conn); ok {
					server.subscribeWS(cc, topic.(string), conn)
				}
			} else {
				_ = conn.WriteJSON(map[string]any{
					"error": "topic missing",
				})
			}

		case "unsub", "unsubscribe":
			if topic, ok := m["topic"]; ok {
				server.unsubscribeWS(topic.(string), conn)
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
			if server.onServerData != nil {
				if data, ok := m["data"]; ok {
					if addr, ok := m["addr"]; ok && strings.Contains(addr.(string), server.Address) {
						server.onServerData(data, conn)
					}
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
						if id.(string) == server.ID {
							if ddd, ok := m["data"].(map[string]any); ok {
								if eventID, ok := ddd["event_id"]; ok {
									server.Publish(eventID.(string), map[string]any{
										"ok":   "done",
										"from": server.ID,
									})
								} else {
									if server.onId != nil {
										server.onId(ddd)
									}
								}
								return
							}
						}
						if from, ok := m["from"]; ok {
							mm["from"] = from
						} else if cc, ok := server.Bus.allWS.Get(conn); ok {
							mm["from"] = cc
						}
						server.PublishToID(id.(string), mm)
					} else {
						_ = conn.WriteJSON(map[string]any{
							"error": "id missing",
						})
					}
				case map[string]any:
					if id, ok := m["id"]; ok {
						if from, ok := m["from"]; ok {
							v["from"] = from
						} else if cc, ok := server.Bus.allWS.Get(conn); ok {
							v["from"] = cc
						}
						if id.(string) == server.ID {
							if eventID, ok := v["event_id"]; ok {
								server.Publish(eventID.(string), map[string]any{
									"ok":   "done",
									"from": server.ID,
								})
							}
							if server.onId != nil {
								server.onId(v)
							}
							return
						}
						server.PublishToID(id.(string), v)
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
		case "pub_server":
			if data, ok := m["data"]; ok {
				switch v := data.(type) {
				case string:
					mm := map[string]any{
						"data": v,
					}
					if addr, ok := m["addr"].(string); ok {
						if strings.Contains(addr, server.Address) {
							server.onServerData(mm, conn)
							return
						}
					}
					if secure, ok := m["secure"]; ok && secure.(bool) {
						err := server.PublishToServer(m["addr"].(string), mm, true)
						if err != nil {
							_ = conn.WriteJSON(map[string]any{
								"error": err.Error(),
							})
						}
					} else {
						err := server.PublishToServer(m["addr"].(string), mm)
						if err != nil {
							_ = conn.WriteJSON(map[string]any{
								"error": err.Error(),
							})
						}
					}
				case map[string]any:
					if addr, ok := m["addr"].(string); ok {
						if strings.Contains(addr, server.Address) {
							server.onServerData(m, conn)
							return
						}
					}
					if secure, ok := m["secure"]; ok && secure.(bool) {
						err := server.PublishToServer(m["addr"].(string), v, true)
						if err != nil {
							_ = conn.WriteJSON(map[string]any{
								"error": err.Error(),
							})
						}
					} else {
						err := server.PublishToServer(m["addr"].(string), v)
						if err != nil {
							_ = conn.WriteJSON(map[string]any{
								"error": err.Error(),
							})
						}
					}
				default:
					_ = conn.WriteJSON(map[string]any{
						"error": "type not handled, only json",
					})
				}
			}
		case "ping":
			var from string
			if from, ok = m["from"].(string); !ok {
				from = GenerateUUID()
			}
			found := false
			for _, v := range server.Bus.allWS.Values() {
				if v == from {
					found = true
				}
			}
			if !found {
				server.Bus.allWS.Set(conn, from)
				server.Bus.idConn.Set(from, conn)
			} else {
				_ = conn.WriteJSON(map[string]any{
					"error": "ID already exist, should be unique",
				})
			}
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
				lg.Info("max retry exceeded", "max", maxRetry)
				break
			}
		} else {
			err = function()
		}
	}
}
