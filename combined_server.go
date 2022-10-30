package kbus

import (
	"fmt"
	"net/url"

	"github.com/kamalshkeir/klog"
	"github.com/kamalshkeir/kmap"
	"github.com/kamalshkeir/kmux"
	"github.com/kamalshkeir/kmux/ws"
)

var localIds = kmap.New[string, struct{}](false)

type CombinedServer struct {
	addr          string
	server        *Server
	serversClient *kmap.SafeMap[AddressOption, *Client]
}

type AddressOption struct {
	Address      string
	Secure       bool
	Distributed  bool
	LoadBalanced bool
	Path         string
}

func connectToServers(addrs ...AddressOption) *kmap.SafeMap[AddressOption, *Client] {
	ret := kmap.New[AddressOption, *Client](false)
	for _, adr := range addrs {
		sch := "ws"
		if adr.Secure {
			sch = "wss"
		}
		spath := ServerPath
		if adr.Path != spath && adr.Path != "" {
			spath = adr.Path
		}
		u := url.URL{Scheme: sch, Host: adr.Address, Path: spath}
		c, resp, err := ws.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			if err == ws.ErrBadHandshake {
				klog.Printf("rdhandshake failed with status %d \n", resp.StatusCode)
			} else if err == ws.ErrCloseSent {
				klog.Printf("rdserver connection closed with status %d \n", resp.StatusCode)
			} else {
				klog.Printf("rddial:%v\n", err)
			}
			continue
		}
		client := &Client{
			serverAddr: u.String(),
			conn:       c,
			done:       make(chan struct{}),
		}
		ret.Set(adr, client)
	}
	return ret
}

func NewCombinedServer(newAddress string,secure bool, serversAddrs ...AddressOption) *CombinedServer {
	serversclients := connectToServers(serversAddrs...)
	server := NewServer()
	c := CombinedServer{
		addr:          newAddress,
		server:        server,
		serversClient: serversclients,
	}
	LocalAddress = newAddress
	for _, srvAddr := range serversAddrs {
		c.subscribeToServer(srvAddr)
	}
	return &c
}

func (c *CombinedServer) subscribeToServer(addr AddressOption) {
	c.serversClient.Range(func(addrOption AddressOption, client *Client) {
		if addrOption.Address == addr.Address {
			if DEBUG {
				klog.Printfs("grSubscribing To Server %s\n", addr)
			}
			if addr.Distributed {
				client.sendDataToServer(addr.Address+":server", map[string]any{
					"action": "server_sub",
					"addr":   LocalAddress,
					"role":"keepServing",
				})
			} else if addr.LoadBalanced {
				client.sendDataToServer(addr.Address+":server", map[string]any{
					"action": "server_sub",
					"addr":   LocalAddress,
					"role":   "publishOnly",
				})
			}
			client.sendDataToServer(addr.Address+":server", map[string]any{
				"action": "server_sub",
				"addr":   LocalAddress,
				"role":   "publishOnly",
				// "role":"keepServing",
			})
		}
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
				if err != nil && DEBUG {
					klog.Printf("rd%v\n", err)
				}
				s.server.removeWS(c.Ws)
				break
			}

			if DEBUG {
				klog.Printfs("--------------------------------\n")
				klog.Printfs("%v\n", m)
			}
			fromPublisher := ""
			if v, ok := m["from_publisher"]; ok {
				if v != "" {
					fromPublisher = v.(string)
				}
			}

			if id, ok := m["id"]; ok {
				if _, ok := localIds.Get(id.(string)); ok {
					if topicAny, ok := m["topic"]; ok {
						if v, ok := m["name"]; ok {
							if topic, ok := topicAny.(string); ok {
								if nm, ok := v.(string); ok {
									if !kmux.SliceContains(localTopics.Keys(), topic, topic+":"+nm) {
										serversTopics.Range(func(addr string, topics map[string]bool) {
											_, ok1 := topics[topic]
											if ok1 {
												if client, ok := subscribedServers.Get(addr); ok {
													client.sendDataToServer(addr, m)
													serversTopics.Range(func(key string, value map[string]bool) {
														if key == addr {
															if len(value) == 0 {
																value = map[string]bool{topic: true}
															} else {
																value[topic] = true
															}
															serversTopics.Set(key, value)
														}
													})
												}
											}
											_, ok2 := topics[topic+":"+nm]
											if ok2 {
												if client, ok := subscribedServers.Get(addr); ok {
													client.sendDataToServer(addr, m)
													serversTopics.Range(func(key string, value map[string]bool) {
														if key == addr {
															if len(value) == 0 {
																value = map[string]bool{topic + ":" + nm: true}
															} else {
																value[topic+":"+nm] = true
															}
															serversTopics.Set(key, value)
														}
													})
												}
											}
										})
									}
								}
							}
						} else {
							if topic, ok := topicAny.(string); ok {
								if _, ok := localTopics.Get(topic); !ok {
									serversTopics.Range(func(addr string, topics map[string]bool) {
										_, ok1 := topics[topic]
										if ok1 {
											if client, ok := subscribedServers.Get(addr); ok {
												client.sendDataToServer(addr, m)
												serversTopics.Range(func(key string, value map[string]bool) {
													if key == addr {
														if len(value) == 0 {
															value = map[string]bool{topic: true}
														} else {
															value[topic] = true
														}
														serversTopics.Set(key, value)
													}
												})
											}
										}
									})
								}
							}
						}
					}
				} else {
					localIds.Set(id.(string), struct{}{})
				}
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
					if topic, ok := m["topic"]; ok {
						if topicString, ok := topic.(string); ok {
							if id, ok := m["id"]; ok {
								s.server.addWS(id.(string), topicString, c.Ws)
							} else {
								fmt.Println("id not found, will not be added:", m)
								continue
							}
							if fromPublisher == "" {
								localTopics.Set(topicString, true)
							}

							if v, ok := m["name"]; ok {
								if id, ok := m["id"]; ok {
									addNamedWS(topicString, v.(string), id.(string), c.Ws)
								} else {
									fmt.Println("id not found, will not be added:", m)
									continue
								}
								if fromPublisher == "" {
									localTopics.Set(m["topic"].(string)+":"+v.(string), true)
								}
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
								continue
							}
							if nn, ok := m["name"]; ok {
								mWSName.Range(func(key ClientSubscription, value []string) {
									for i, v := range value {
										if id, ok := m["id"]; ok {
											if topic+":"+v == nn || v == nn || key.Id == id {
												value = append(value[:i], value[i+1:]...)
												mWSName.Set(key, value)
												localTopics.Delete(nn.(string))
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
					if topic, ok := m["topic"]; ok {
						s.server.RemoveTopic(topic.(string))
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
										if _, ok := localTopics.Get(topic + ":" + nn); ok {
											s.server.SendTo(topic+":"+nn, mm)
										} else if _, ok := localTopics.Get(nn); ok {
											s.server.SendTo(topic+":"+nn, mm)
										} else {
											serversTopics.Range(func(addr string, topics map[string]bool) {
												if _, ok := topics[topic]; ok {
													if client, ok := subscribedServers.Get(addr); ok {
														client.SendTo(topic, m)
													}
												}
											})
										}
									} else {
										if _, ok := localTopics.Get(nn); ok {
											s.server.SendTo(nn, mm)
										} else {
											serversTopics.Range(func(addr string, topics map[string]bool) {
												if _, ok := topics[topic]; ok {
													if client, ok := subscribedServers.Get(addr); ok {
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
									if _, ok := localTopics.Get(topic + ":" + name.(string)); ok {
										s.server.SendTo(topic+":"+name.(string), v)
									} else if _, ok := localTopics.Get(name.(string)); ok {
										s.server.SendTo(topic+":"+name.(string), v)
									} else {
										serversTopics.Range(func(addr string, topics map[string]bool) {
											if _, ok := topics[topic+":"+name.(string)]; ok {
												if client, ok := subscribedServers.Get(addr); ok {
													client.SendTo(topic+":"+name.(string), m)
												}
											}
										})
									}
								} else {
									if _, ok := localTopics.Get(name.(string)); ok {
										s.server.SendTo(name.(string), v)
									} else if _, ok := localTopics.Get(name.(string)); ok {
										s.server.SendTo(name.(string), v)
									} else {
										serversTopics.Range(func(addr string, topics map[string]bool) {
											if _, ok := topics[name.(string)]; ok {
												if client, ok := subscribedServers.Get(addr); ok {
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
					if topicsRes, ok = serversTopics.Get(m["addr"].(string)); !ok {
						topicsRes = map[string]bool{}
					}

					if v, ok := m["topics"]; ok {
						for _, tpIN := range v.([]any) {
							if len(topicsRes) == 0 {
								topicsRes = map[string]bool{tpIN.(string): true}
							} else {
								topicsRes[tpIN.(string)] = true
							}
						}
					}

					serversTopics.Set(m["addr"].(string), topicsRes)

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
	})
}
