"""
This module implements a ksbus client https://github.com/kamalshkeir/ksbus

Author: Kamal Shkeir
Email: kamalshkeir@gmail.com
Link: https://kamalshkeir.dev
"""

import asyncio
import json
import random
import string

import websockets


class Bus:
    def __init__(self, options, block=False):
        self.addr = options.get('addr', 'localhost')
        self.path = options.get('path', '/ws/bus')
        self.scheme = 'ws://'
        if options.get('secure', False):
            self.scheme = 'wss://'
        self.full_address = self.scheme + self.addr + self.path
        self.conn = None
        self.topic_handlers = {}
        self.autorestart = options.get('autorestart', False)
        self.restartevery = options.get('restartevery', 5)
        self.OnOpen = options.get('OnOpen', lambda bus: None)
        self.OnClose = options.get('OnClose', lambda: None)
        self.OnDataWs = options.get('OnDataWs', None)
        self.OnId = options.get('OnId', lambda data: None)
        self.id = options.get('id') or self.makeId(12)
        try:
            if block :
                asyncio.get_event_loop().run_until_complete(self.connect(self.full_address))
            else:
                asyncio.create_task(self.connect(self.full_address))
        except Exception as e:
            print(e)
    
    async def connect(self, path):
        try:
            self.conn = await websockets.connect(path)
            await self.sendMessage({"action": "ping", "from": self.id})
            async for message in self.conn:
                obj = json.loads(message)
                if self.OnDataWs is not None:
                    self.OnDataWs(obj)
                if "event_id" in obj:
                    self.Publish(obj["event_id"], {"ok": "done", "from": self.id, "event_id":obj["event_id"]})
                if "to_id" in obj:
                    if self.OnId is not None:
                        self.OnId(obj)
                elif "topic" in obj:
                    if obj["topic"] in self.topic_handlers:
                        subs = BusSubscription(self, obj["topic"])
                        self.topic_handlers[obj["topic"]](obj, subs)
                    else:
                        print(f"topicHandler not found for: {obj['topic']}")
                elif "data" in obj and obj["data"] == "pong":
                    if self.OnOpen is not None:
                        self.OnOpen(self)
        except Exception as e:
            print(f"Server closed the connection: {e}")
            if self.autorestart:
                while True:
                    print(f"Reconnecting in {self.restartevery} seconds...")
                    await asyncio.sleep(self.restartevery)
                    await self.connect(self.full_address)

    def Subscribe(self, topic, handler):
        payload = {"action": "sub", "topic": topic, "from": self.id}
        subs = BusSubscription(self, topic)
        self.topic_handlers[topic] = handler
            
        if self.conn is not None:
            asyncio.create_task(self.sendMessage(payload))
        return subs

    def Unsubscribe(self, topic):
        payload = {"action": "unsub", "topic": topic, "from": self.id}
        del self.topic_handlers[topic]
            
        if topic and self.conn is not None:
            asyncio.create_task(self.sendMessage(payload))

    async def sendMessage(self, obj):
        try:
            await self.conn.send(json.dumps(obj))
        except Exception as e:
            print("error sending message:", e)

    def Publish(self, topic, data):
        if self.conn is not None:
            asyncio.create_task(self.sendMessage({"action": "pub", "topic": topic, "data": data, "from": self.id}))
        else:
            print("Publish: Not connected to server. Please check the connection.")

    def PublishToID(self, topic, data):
        if self.conn is not None:
            asyncio.create_task(self.sendMessage({"action": "pub_id", "id": self.id, "data": data, "from": self.id}))
        else:
            print("PublishToID: Not connected to server. Please check the connection.")

    def RemoveTopic(self, topic):
        if self.conn is not None:
            asyncio.create_task(self.sendMessage({"action": "remove", "topic": topic, "from": self.id}))
            del self.topic_handlers[topic]

    def makeId(self, length):
        return "".join(random.choices(string.ascii_letters + string.digits, k=length))

    def PublishWaitRecv(self, topic, data, onRecv, onExpire):
        data["from"] = self.id
        data["topic"] = topic
        eventId = self.makeId(8)
        data["event_id"] = eventId
        done = False

        self.Subscribe(eventId, lambda data, ch: self._onRecv(data, ch, onRecv, done))
        self.Publish(topic, data)
        asyncio.create_task(self._setExpireTimer(eventId, onExpire))

    async def _setExpireTimer(self, eventId, onExpire):
        await asyncio.sleep(0.5)
        if not self.topic_handlers.get(eventId):
            if onExpire:
                onExpire(eventId)

    def _onRecv(self, data,ch, onRecv, done):
        if not done:
            done = True
            if onRecv:
                onRecv(data)
                ch.Unsubscribe()

    def PublishToIDWaitRecv(self, id, data, onRecv, onExpire):
        data["from"] = self.id
        data["id"] = id
        eventId = self.makeId(8)
        data["event_id"] = eventId
        done = False

        self.Subscribe(eventId, lambda data, ch: self._onRecv(data, ch, onRecv, done))
        self.PublishToID(id, data)
        asyncio.create_task(self._setExpireTimer(eventId, onExpire))

    def PublishToServer(self, addr, data, secure):
        self.conn.send(json.dumps({
            "action": "pub_server",
            "addr": addr,
            "data": data,
            "secure": secure,
            "from": self.id
        }))


class BusSubscription:
    def __init__(self, bus, topic):
        self.bus = bus
        self.topic = topic

    async def sendMessage(self, obj):
        try:
            await self.bus.conn.send(json.dumps(obj))
        except Exception as e:
            print("error sending message:", e)

    def Unsubscribe(self):
        asyncio.create_task(self.sendMessage({"action": "unsub", "topic": self.topic, "from": self.bus.id}))
        del self.bus.topic_handlers[self.topic]

