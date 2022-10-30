package kbus

import (
	"fmt"
	"time"

	"github.com/kamalshkeir/klog"
	"github.com/kamalshkeir/kmap"
	"github.com/kamalshkeir/kmux"
	"github.com/kamalshkeir/kmux/ws"
)

var (
	keepServing = false
	publishOnly = false
	subscribedServers = kmap.New[string,*Client](false)
	localTopics = kmap.New[string,bool](false)
	serversTopics = kmap.New[string,map[string]bool](false)
)


func (s *Server) publishWS(topic string, data map[string]any) {
	go func() {
		s.Bus.mu.Lock()
		defer s.Bus.mu.Unlock()	
		data["topic"]=topic
		if subscriptions, ok := s.Bus.wsSubscribers.Get(topic); ok {
			for _, sub := range subscriptions {
				_ = sub.Conn.WriteJSON(data)
			}
		}	
	}()
}

func (s *Server) addWS(id,topic string, conn *ws.Conn) {
	wsSubscribers := s.Bus.wsSubscribers
	new := ClientSubscription{
		Id: GenerateRandomString(5),
		Conn: conn,
	}
	clients, ok := wsSubscribers.Get(topic)
	if ok {
		if len(clients) == 0 {
			clients = []ClientSubscription{new}
			wsSubscribers.Set(topic, clients)
		} else {
			found := false
			for _,c := range clients {
				if c.Conn == conn {
					found=true
				}
			}
			if !found {
				clients = append(clients, new)
				wsSubscribers.Set(topic, clients)
			} 
		}
	} else {
		clients=[]ClientSubscription{new}
		wsSubscribers.Set(topic, clients)
	}
}

func (s *Server) removeWS(wsConn *ws.Conn) {
	mWSName.Range(func(key ClientSubscription, value []string) {
		if key.Conn == wsConn {
			mWSName.Delete(key)
		}
	})
	s.Bus.wsSubscribers.Range(func(key string, value []ClientSubscription) {
		for i,sub := range value {
			if sub.Conn == wsConn {
				value = append(value[:i], value[i+1:]...)
				s.Bus.wsSubscribers.Set(key, value)
			}
		}
	})	
}

func (s *Server) unsubscribeWS(id,topic string, wsConn *ws.Conn) {
	s.Bus.wsSubscribers.Range(func(key string, value []ClientSubscription) {
		for i, sub := range value {
			if sub.Conn == wsConn {
				value = append(value[:i], value[i+1:]...)
				s.Bus.wsSubscribers.Set(key, value)
			}
		}
	})
	mWSName.Range(func(key ClientSubscription, value []string) {
		if key.Conn == wsConn {
			mWSName.Delete(key)
		}
	})
}


func (server *Server) AllTopics() []string {
	res := make(map[string]struct{})
	server.Bus.subscribers.Range(func(key string, value []Channel) {
		res[key]=struct{}{}
	})
	server.Bus.wsSubscribers.Range(func(key string, value []ClientSubscription) {
		res[key]=struct{}{}
	})
	mWSName.Range(func(key ClientSubscription, value []string) {
		for _,v := range value {
			res[v]=struct{}{}
		}
	})
	n := []string{}
	for k := range res {
		n = append(n, k)
	}
	return n
}

func (server *Server) handleWS(addr string) {
	ws.FuncBeforeUpgradeWS=BeforeUpgradeWS
	server.App.WS(ServerPath, func(c *kmux.WsContext) {
		for {
			m, err := c.ReceiveJson()
			if err != nil || !BeforeDataWS(m,c.Ws,c.Request) {
				if err != nil && DEBUG {
					klog.Printf("rd%v\n",err)
				}
				server.removeWS(c.Ws)
				break
			}
		
			if keepServing {			
				if topic,ok := m["topic"];ok {
					if _,ok := localTopics.Get(topic.(string));!ok {
						subscribedServers.Range(func(addr string, client *Client) {
							m["from_publisher"]=LocalAddress
							client.sendDataToServer(addr,m)
						})
						continue
					}
				}
			} else if publishOnly {
				subscribedServers.Range(func(addr string, client *Client) {
					m["from_publisher"]=LocalAddress
					client.sendDataToServer(addr,m)
				})
				continue
			}

			if DEBUG {
				klog.Printfs("--------------------------------\n")
				klog.Printfs("yl%v\n",m)
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
						if id,ok := m["id"];ok {
							server.addWS(id.(string),topic.(string), c.Ws)
							localTopics.Set(topic.(string),true)
						} else {
							fmt.Println("id not found, will not be added:",m)
						}
						if v,ok := m["name"];ok {
							if id,ok := m["id"];ok {
								addNamedWS(topic.(string),v.(string),id.(string),c.Ws)
								localTopics.Set(topic.(string)+":"+v.(string),true)
							} else {
								fmt.Println("id not found, will not be added:",m)
							}
						} 
					} else {
						c.Json(map[string]any{
							"error": "topic missing",
						})
					}		
				case "unsub", "unsubscribe":
					if topic, ok := m["topic"]; ok {
						if id,ok := m["id"];ok {
							server.unsubscribeWS(id.(string),topic.(string), c.Ws)
						} else {
							fmt.Println("id not found, will not be removed:",m)
						}
						if nn,ok := m["name"];ok {
							mWSName.Range(func(key ClientSubscription, value []string) {
								for i,v := range value {
									if id,ok := m["id"];ok {
										if topic.(string)+":"+v == nn || v == nn || key.Id == id {
											value = append(value[:i],value[i+1:]... )
											mWSName.Set(key,value)
										}
									} else {
										fmt.Println("id not found, will not be removed:",m)
									}
								}
							})
						}
					} else {
						c.Json(map[string]any{
							"error": "topic missing",
						})
					}
				case "remove_topic", "removeTopic":
					if topic, ok := m["topic"]; ok {
						server.RemoveTopic(topic.(string))
					} else {
						c.Json(map[string]any{
							"error": "topic missing",
						})
					}
				case "send", "sendTo":
					if data, ok := m["data"]; ok {
						topic := ""
						if top,ok := m["topic"];ok && top != "" {
							topic=top.(string)
						}
						switch v := data.(type) {
						case string:
							mm := map[string]any{
								"data": v,
							}
							if name, ok := m["name"]; ok {
								if nn,ok :=name.(string);ok {
									if topic != "" {
										server.SendTo(topic+":"+nn, mm)
									} else {
										server.SendTo(nn, mm)
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
									server.SendTo(topic+":"+name.(string), v)
								} else {
									server.SendTo(name.(string), v)
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
					if role,ok := m["role"];ok {
						switch role {
						case "publishOnly","publish_only":
							publishOnly=true
						case "keep_serving","subscriber","keepServing":
							keepServing=true
						}
						client,err := NewClient(m["addr"].(string),false)
						klog.CheckError(err)
						subscribedServers.Set(m["addr"].(string),client)
						fmt.Println("subscribed servers:",subscribedServers.Keys())
						topics := server.AllTopics()
						for _,t :=range topics {
							localTopics.Set(t,true)
						}
						client.sendDataToServer(m["addr"].(string)+":server",map[string]any{
							"action":"topics",
							"addr":LocalAddress,
							"topics":topics,
						})
					}
				case "ping":
					_ = c.Json(map[string]any{
						"data":"pong",
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

func setName(ch Channel, name string) {
	ch.Name=name
	if ch.Topic != "" && ch.Name != "" {
		ch.Name=ch.Topic+":"+ch.Name	
	} 
	if ch.Id == "" {
		ch.Id=GenerateRandomString(5)
	}
	if v,ok := mChannelName.Get(ch);ok {
		if !kmux.SliceContains(v,ch.Name) {
			v = append(v, ch.Name)
			mChannelName.Set(ch,v)
		} 
	} else {
		mChannelName.Set(ch,[]string{ch.Name})
	}
}

func addNamedWS(topic,name,id string, conn *ws.Conn) {
	clientSub := ClientSubscription{
		Id: id,
		Name: name,
		Topic: topic,
		Conn: conn,
	}
	if v,ok := mWSName.Get(clientSub);ok {
		v = append(v, topic+":"+name)
		mWSName.Set(ClientSubscription{Conn: conn,Id: id,Topic: topic,Name: name}, v)
	} else {
		mWSName.Set(ClientSubscription{Conn: conn,Id: id,Topic: topic,Name: name}, []string{topic+":"+name})
	}
}


// Cronjob like
func RunEvery(t time.Duration,fn func() bool) {
	fn()
	c := time.NewTicker(t)
	for range c.C {
		if(fn()) {
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