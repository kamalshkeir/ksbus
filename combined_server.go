package kbus

import (
	"fmt"

	"github.com/kamalshkeir/klog"
	"github.com/kamalshkeir/kmux"
	"github.com/kamalshkeir/kmux/ws"
)



type CombinedServer struct {
	addr          string
	server        *Server
}

type AddressOption struct {
	Address      string
	Secure       bool
	Path         string
}


func NewCombinedServer(newAddress string,secure bool, serversAddrs ...AddressOption) *CombinedServer {
	server := NewServer()
	c := CombinedServer{
		addr:          newAddress,
		server:        server,
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
	conn := c.server.sendData(addr.Address,map[string]any{
		"action": "server_sub",
		"addr":   LocalAddress,
		"secure":addr.Secure,
	},addr.Secure)
	mSubscribedServers.Set(addr.Address,&Client{
		conn: conn,
		id: GenerateRandomString(7),
		serverAddr: addr.Address,
	})
}

func (s *CombinedServer) Subscribe(topic string, fn func(data map[string]any, ch Channel), name ...string) (ch Channel) {
	return s.server.Subscribe(topic, fn, name...)
}

func (s *CombinedServer) Unsubscribe(topic string, ch Channel) {
	if ch.Ch != nil {
		s.server.Unsubscribe(topic, ch)
	}
}

func (s *CombinedServer) SendTo(name string, data map[string]any) {
	s.server.SendTo(name, data)
}

func (s *CombinedServer) Publish(topic string, data map[string]any) {
	s.server.Publish(topic, data)
}

func (s *CombinedServer) RemoveTopic(topic string) {
	s.server.RemoveTopic(topic)
}

// RUN
func (s *CombinedServer) Run() {
	s.handleWS(LocalAddress)
	s.server.App.Run(LocalAddress)
}

func (s *CombinedServer) RunTLS(cert string, certKey string) {
	s.handleWS(LocalAddress)
	s.server.App.RunTLS(LocalAddress, cert, certKey)
}

func (s *CombinedServer) RunAutoTLS(subDomains ...string) {
	s.handleWS(LocalAddress)
	s.server.App.RunAutoTLS(LocalAddress, subDomains...)
}

func (s *CombinedServer) handleWS(addr string) {
	ws.FuncBeforeUpgradeWS = BeforeUpgradeWS
	s.server.App.WS(ServerPath, func(c *kmux.WsContext) {
		for {
			m, err := c.ReceiveJson()
			if err != nil || !BeforeDataWS(m, c.Ws, c.Request) {
				s.server.removeWS(c.Ws)
				break
			}

			if DEBUG {
				klog.Printfs("--------------------------------\n")
				klog.Printfs("yl%v\n", m)
			}

			
			go s.handleActions(m,c)	
		}
	})
}

func (s *CombinedServer) handleActions(m map[string]any,c *kmux.WsContext) {
	publisher := ""
	if publisherAddr,ok := m["from_publisher"];ok {
		if pubAddr,ok := publisherAddr.(string);ok {
			publisher=pubAddr
		}
	}
	if publisher != "master" && publisher != LocalAddress && publisher != "localhost"+LocalAddress {
		mServersTopics.Range(func(addr string, tpcs map[string]bool) {
			if publisher != addr {
				m["from_publisher"]="master"
				fmt.Println("Combined sending data to ",addr,m)
				s.server.sendData(addr,m)
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
						s.server.Publish(tpc.(string), mm)
					} else {
						c.Json(map[string]any{
							"error": "topic missing",
						})
					}
				case map[string]any:
					if topic, ok := m["topic"]; ok {
						s.server.Publish(topic.(string), v)
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
						s.server.Bus.mu.Lock()
						s.server.addWS(id.(string), topicString, c.Ws)
						s.server.Bus.mu.Unlock()
					} else {
						fmt.Println("id not found, will not be added:", m)
					}
					mLocalTopics.Set(topicString, true)

					if v, ok := m["name"]; ok {
						if id, ok := m["id"]; ok {
							addNamedWS(topicString, v.(string), id.(string), c.Ws)
						} else {
							fmt.Println("id not found, will not be added:", m)
						}
						mLocalTopics.Set(m["topic"].(string)+":"+v.(string), true)
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
						s.server.unsubscribeWS(id.(string), topic, c.Ws)
					} else {
						fmt.Println("id not found, will not be removed:", m)
					}
					if nn, ok := m["name"]; ok {
						mWSName.Range(func(key ClientSubscription, value []string) {
							for i, v := range value {
								if id, ok := m["id"]; ok {
									if topic+":"+v == nn || v == nn || key.Id == id {
										value = append(value[:i], value[i+1:]...)
										mWSName.Set(key, value)
										mLocalTopics.Delete(nn.(string))
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
			if topic, ok := m["topic"]; ok && publisher == ""{
				s.server.RemoveTopic(topic.(string))
			} else if publisher != "" {
				mServersTopics.Range(func(addr string, value map[string]bool) {
					if addr == publisher {
						value[m["topic"].(string)]=true
						mServersTopics.Set(addr,value)
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
								if _, ok := mLocalTopics.Get(topic + ":" + nn); ok {
									s.server.SendTo(topic+":"+nn, mm)
								} else if _, ok := mLocalTopics.Get(nn); ok {
									s.server.SendTo(topic+":"+nn, mm)
								} else {
									mServersTopics.Range(func(addr string, topics map[string]bool) {
										if _, ok := topics[topic]; ok {
											if client, ok := mSubscribedServers.Get(addr); ok {
												client.SendTo(topic, m)
											}
										}
									})
								}
							} else {
								if _, ok := mLocalTopics.Get(nn); ok {
									s.server.SendTo(nn, mm)
								} else {
									mServersTopics.Range(func(addr string, topics map[string]bool) {
										if _, ok := topics[topic]; ok {
											if client, ok := mSubscribedServers.Get(addr); ok {
												client.SendTo(topic, m)
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
							if _, ok := mLocalTopics.Get(topic + ":" + name.(string)); ok {
								s.server.SendTo(topic+":"+name.(string), v)
							} else if _, ok := mLocalTopics.Get(name.(string)); ok {
								s.server.SendTo(topic+":"+name.(string), v)
							} else {
								mServersTopics.Range(func(addr string, topics map[string]bool) {
									if _, ok := topics[topic+":"+name.(string)]; ok {
										if client, ok := mSubscribedServers.Get(addr); ok {
											client.SendTo(topic+":"+name.(string), m)
										}
									}
								})
							}
						} else {
							if _, ok := mLocalTopics.Get(name.(string)); ok {
								s.server.SendTo(name.(string), v)
							} else if _, ok := mLocalTopics.Get(name.(string)); ok {
								s.server.SendTo(name.(string), v)
							} else {
								mServersTopics.Range(func(addr string, topics map[string]bool) {
									if _, ok := topics[name.(string)]; ok {
										if client, ok := mSubscribedServers.Get(addr); ok {
											client.SendTo(name.(string), m)
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
			if topicsRes, ok = mServersTopics.Get(m["addr"].(string)); !ok {
				topicsRes = map[string]bool{}
			}

			if v, ok := m["topics"]; ok {
				topicsRes = map[string]bool{}
				for _, tpIN := range v.([]any) {
					topicsRes[tpIN.(string)]=true
				}
			}
			mServersTopics.Set(m["addr"].(string), topicsRes)

		case "remove_node_topic":
			addrr := m["addr"]
			tp := m["topic_to_delete"]
			mServersTopics.Range(func(addr string, topics map[string]bool) {
				if addr == addrr.(string) {
					delete(topics,tp.(string))
					mServersTopics.Set(addr,topics)
				}
			})
		case "new_node":				
			secur := false
			if vAny,ok := m["secure"];ok {
				if vAny.(bool) {
					secur=true
				}
			}
			client,err := NewClient(m["addr"].(string),secur)
			if !klog.CheckError(err) {
				mServersTopics.Set(m["addr"].(string),map[string]bool{})
				mSubscribedServers.Set(m["addr"].(string),client)
			}
		case "server_message","serverMessage":
			if data,ok := m["data"];ok {
				if addr,ok := m["addr"];ok && addr.(string) == LocalAddress {
					BeforeServersData(data,c.Ws)
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
