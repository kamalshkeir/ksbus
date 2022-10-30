package kbus

import (
	"net/http"

	"github.com/kamalshkeir/kmap"
	"github.com/kamalshkeir/kmux/ws"
)


type M map[string]any

var (
	DEBUG = false
	LOG=false
	ServerPath="/ws/bus"
	LocalAddress=""
	mChannelName = kmap.New[Channel, []string](false)
	mWSName = kmap.New[ClientSubscription, []string](false)
	CLIENT_SECURE = false
	mClientTopicHandlers = kmap.New[string,func(map[string]any,*ClientSubscription)](false)
	BeforeDataWS = func(data map[string]any,conn *ws.Conn, originalRequest *http.Request) bool {
		return true
	}
	BeforeUpgradeWS=func(r *http.Request) bool {
		return true
	}
)


