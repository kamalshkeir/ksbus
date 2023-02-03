package ksbus

import (
	"fmt"

	"github.com/kamalshkeir/klog"
	"github.com/kamalshkeir/kmap"
	"github.com/kamalshkeir/kmux"
	"github.com/kamalshkeir/kmux/ws"
)

type CombinedServer struct {
	Address       string
	Server        *Server
	serversTopics *kmap.SafeMap[string, map[string]bool]
}

type AddressOption struct {
	Address string
	Secure  bool
	Path    string
}

func NewCombinedServer(newAddress string, secure bool, serversAddrs ...AddressOption) *CombinedServer {
	server := NewServer()
	c := CombinedServer{
		Address:       newAddress,
		Server:        server,
		serversTopics: kmap.New[string, map[string]bool](false),
	}
	LocalAddress = newAddress
	for _, srvAddr := range serversAddrs {
		c.subscribeToServer(srvAddr)
	}

	return &c
}

func (c *CombinedServer) subscribeToServer(addr AddressOption) {
	if DEBUG {
		klog.Printfs("grSubscribing To Server %v\n", addr)
	}
	conn := c.Server.sendData(addr.Address, map[string]any{
		"action": "server_sub",
		"addr":   LocalAddress,
		"secure": addr.Secure,
	}, addr.Secure)
	c.Server.subscribedServersClients.Set(addr.Address, &Client{
		Conn:       conn,
		Id:         GenerateRandomString(7),
		ServerAddr: addr.Address,
	})
}

func (s *CombinedServer) Subscribe(topic string, fn func(data map[string]any, ch Channel), name ...string) (ch Channel) {
	return s.Server.Subscribe(topic, fn, name...)
}

func (s *CombinedServer) Unsubscribe(topic string, ch Channel) {
	if ch.Ch != nil {
		s.Server.Unsubscribe(topic, ch)
	}
}

func (s *CombinedServer) SendToNamed(name string, data map[string]any) {
	s.Server.SendToNamed(name, data)
}

func (s *CombinedServer) Publish(topic string, data map[string]any) {
	s.Server.Publish(topic, data)
}

func (s *CombinedServer) RemoveTopic(topic string) {
	s.Server.RemoveTopic(topic)
}

// RUN
func (s *CombinedServer) Run() {
	s.handleWS(LocalAddress)
	s.Server.App.Run(LocalAddress)
}

func (s *CombinedServer) RunTLS(cert string, certKey string) {
	s.handleWS(LocalAddress)
	s.Server.App.RunTLS(LocalAddress, cert, certKey)
}

func (s *CombinedServer) RunAutoTLS(subDomains ...string) {
	s.handleWS(LocalAddress)
	s.Server.App.RunAutoTLS(LocalAddress, subDomains...)
}

func (s *CombinedServer) handleWS(addr string) {
	ws.FuncBeforeUpgradeWS = BeforeUpgradeWS
	s.Server.App.WS(ServerPath, func(c *kmux.WsContext) {
		for {
			m, err := c.ReceiveJson()
			if err != nil || !BeforeDataWS(m, c.Ws, c.Request) {
				s.Server.removeWS(c.Ws)
				break
			}

			if DEBUG {
				klog.Printfs("--------------------------------\n")
				klog.Printfs("yl%v\n", m)
			}

			go s.handleActions(m, c)
		}
	})
}

func (s *CombinedServer) handleActions(m map[string]any, c *kmux.WsContext) {
	publisher := ""
	if publisherAddr, ok := m["from_publisher"]; ok {
		if pubAddr, ok := publisherAddr.(string); ok {
			publisher = pubAddr
		}
	}
	if publisher != "master" && publisher != LocalAddress && publisher != "localhost"+LocalAddress {
		s.serversTopics.Range(func(addr string, tpcs map[string]bool) {
			if publisher != addr {
				m["from_publisher"] = "master"
				fmt.Println("Combined sending data to ", addr, m)
				s.Server.sendData(addr, m)
			}
		})
	}

	if action, ok := m["action"]; ok {
		switch action {
		case "pub", "publish":
			if data, ok := m["data"]; ok {
				switch v := data.(type) {
				case string:
					mm := map[string]any{
						"data": v,
					}
					if tpc, ok := m["topic"]; ok {
						s.Server.Publish(tpc.(string), mm)
					} else {
						c.Json(map[string]any{
							"error": "topic missing",
						})
					}
				case map[string]any:
					if topic, ok := m["topic"]; ok {
						s.Server.Publish(topic.(string), v)
					} else {
						c.Json(map[string]any{
							"error": "topic missing",
						})
					}
				default:
					c.Json(map[string]any{
						"error": "type not handled, only json accepted",
					})
				}
			}
		case "sub", "subscribe":
			if topic, ok := m["topic"]; ok && publisher == "" {
				if topicString, ok := topic.(string); ok {
					if id, ok := m["id"]; ok {
						s.Server.Bus.mu.Lock()
						s.Server.addWS(id.(string), topicString, c.Ws)
						s.Server.Bus.mu.Unlock()
					} else {
						fmt.Println("id not found, will not be added:", m)
					}
					s.Server.localTopics.Set(topicString, true)

					if v, ok := m["name"]; ok {
						if id, ok := m["id"]; ok {
							addNamedWS(topicString, v.(string), id.(string), c.Ws)
						} else {
							fmt.Println("id not found, will not be added:", m)
						}
						s.Server.localTopics.Set(m["topic"].(string)+":"+v.(string), true)
					}
				}
			} else {
				c.Json(map[string]any{
					"error": "topic missing",
				})
			}
		case "unsub", "unsubscribe":
			if topc, ok := m["topic"]; ok {
				if topic, ok := topc.(string); ok {
					if id, ok := m["id"]; ok {
						s.Server.unsubscribeWS(id.(string), topic, c.Ws)
					} else {
						fmt.Println("id not found, will not be removed:", m)
					}
					if nn, ok := m["name"]; ok {
						clientSubNames.Range(func(key ClientSubscription, value []string) {
							for i, v := range value {
								if id, ok := m["id"]; ok {
									if topic+":"+v == nn || v == nn || key.Id == id {
										value = append(value[:i], value[i+1:]...)
										go clientSubNames.Set(key, value)
										s.Server.localTopics.Delete(nn.(string))
									}
								} else {
									fmt.Println("id not found, will not be removed:", m)
									continue
								}
							}
						})
					}
				}
			} else {
				c.Json(map[string]any{
					"error": "topic missing",
				})
			}
		case "remove_topic", "removeTopic":
			if topic, ok := m["topic"]; ok && publisher == "" {
				s.Server.RemoveTopic(topic.(string))
			} else if publisher != "" {
				s.serversTopics.Range(func(addr string, value map[string]bool) {
					if addr == publisher {
						value[m["topic"].(string)] = true
						go s.serversTopics.Set(addr, value)
					}
				})
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
								if _, ok := s.Server.localTopics.Get(topic + ":" + nn); ok {
									s.Server.SendToNamed(topic+":"+nn, mm)
								} else if _, ok := s.Server.localTopics.Get(nn); ok {
									s.Server.SendToNamed(topic+":"+nn, mm)
								} else {
									s.serversTopics.Range(func(addr string, topics map[string]bool) {
										if _, ok := topics[topic]; ok {
											if client, ok := s.Server.subscribedServersClients.Get(addr); ok {
												go client.SendToNamed(topic, m)
											}
										}
									})
								}
							} else {
								if _, ok := s.Server.localTopics.Get(nn); ok {
									s.Server.SendToNamed(nn, mm)
								} else {
									s.serversTopics.Range(func(addr string, topics map[string]bool) {
										if _, ok := topics[topic]; ok {
											if client, ok := s.Server.subscribedServersClients.Get(addr); ok {
												go client.SendToNamed(topic, m)
											}
										}
									})
								}
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
							if _, ok := s.Server.localTopics.Get(topic + ":" + name.(string)); ok {
								s.Server.SendToNamed(topic+":"+name.(string), v)
							} else if _, ok := s.Server.localTopics.Get(name.(string)); ok {
								s.Server.SendToNamed(topic+":"+name.(string), v)
							} else {
								s.serversTopics.Range(func(addr string, topics map[string]bool) {
									if _, ok := topics[topic+":"+name.(string)]; ok {
										if client, ok := s.Server.subscribedServersClients.Get(addr); ok {
											go client.SendToNamed(topic+":"+name.(string), m)
										}
									}
								})
							}
						} else {
							if _, ok := s.Server.localTopics.Get(name.(string)); ok {
								s.Server.SendToNamed(name.(string), v)
							} else if _, ok := s.Server.localTopics.Get(name.(string)); ok {
								s.Server.SendToNamed(name.(string), v)
							} else {
								s.serversTopics.Range(func(addr string, topics map[string]bool) {
									if _, ok := topics[name.(string)]; ok {
										if client, ok := s.Server.subscribedServersClients.Get(addr); ok {
											go client.SendToNamed(name.(string), m)
										}
									}
								})
							}
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

		case "topics":
			var topicsRes map[string]bool
			var ok bool
			if topicsRes, ok = s.serversTopics.Get(m["addr"].(string)); !ok {
				topicsRes = map[string]bool{}
			}

			if v, ok := m["topics"]; ok {
				topicsRes = map[string]bool{}
				for _, tpIN := range v.([]any) {
					topicsRes[tpIN.(string)] = true
				}
			}
			s.serversTopics.Set(m["addr"].(string), topicsRes)

		case "remove_node_topic":
			addrr := m["addr"]
			tp := m["topic_to_delete"]
			s.serversTopics.Range(func(addr string, topics map[string]bool) {
				if addr == addrr.(string) {
					delete(topics, tp.(string))
					go s.serversTopics.Set(addr, topics)
				}
			})
		case "new_node":
			secur := false
			if vAny, ok := m["secure"]; ok {
				if vAny.(bool) {
					secur = true
				}
			}
			client := NewClient()
			err := client.Connect(m["addr"].(string), secur)
			if !klog.CheckError(err) {
				s.serversTopics.Set(m["addr"].(string), map[string]bool{})
				s.Server.subscribedServersClients.Set(m["addr"].(string), client)
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
