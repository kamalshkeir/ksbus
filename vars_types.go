package ksbus

import (
	"net/http"

	"github.com/kamalshkeir/kmap"
	"github.com/kamalshkeir/kmux/ws"
)


type M map[string]any

var (
	DEBUG = false
	CLIENT_SECURE = false
	ServerPath="/ws/bus"
	LocalAddress=""
	mChannelName = kmap.New[Channel, []string](false)
	mWSName = kmap.New[ClientSubscription, []string](false)
	mClientTopicHandlers = kmap.New[string,func(map[string]any,*ClientSubscription)](false)
	mServersConnectionsSendToServer = kmap.New[string,*ws.Conn](false)
	mSubscribedServers = kmap.New[string,*Client](false)
	mLocalTopics = kmap.New[string,bool](false)
	mServersTopics = kmap.New[string,map[string]bool](false)
	BeforeDataWS = func(data map[string]any,conn *ws.Conn, originalRequest *http.Request) bool {
		return true
	}
	BeforeServersData = func(data any,conn *ws.Conn) {}
	BeforeUpgradeWS=func(r *http.Request) bool {
		return true
	}
)


