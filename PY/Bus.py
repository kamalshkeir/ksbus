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
    def __init__(self, addr, path="/ws/bus", secure=False,onOpen=lambda bus: None):
        self.scheme = "ws://"
        if secure:
            self.scheme = "wss://"
        self.path = self.scheme + addr + path
        self.conn = None
        self.topic_handlers = {}
        self.autorestart = False
        self.restartevery = 10
        self.OnOpen = onOpen
        self.OnData = lambda data: None
        self.on_close = lambda: None
        self.id = self.makeid(8)
        try:
            asyncio.get_event_loop().run_until_complete(self.connect(self.path))
        except Exception as e:
            print(e)

    async def connect(self, path):
        async with websockets.connect(path) as ws:
            self.conn=ws
            await self.sendMessage({"action": "ping", "id": self.id})
            try:
                async for message in self.conn:
                    obj = json.loads(message)
                    if "topic" in obj:
                        if obj["topic"] in self.topic_handlers:
                            subs = BusSubscription(self, obj["topic"])
                            if "name" in obj:
                                subs = BusSubscription(self, obj["topic"], obj["name"])
                            await self.topic_handlers[obj["topic"]](obj, subs)
                        else:
                            self.OnData(obj)
                            print(f"topicHandler not found for: {obj['topic']}")
                    elif "name" in obj:
                        if obj["name"] in self.topic_handlers:
                            subs = BusSubscription(self, obj["topic"])
                            if "topic" in obj:
                                subs = BusSubscription(self, obj["topic"], obj["name"])
                            await self.topic_handlers[obj["name"]](obj, subs)
                        else:
                            self.OnData(obj)
                            print(f"topicHandler not found for: {obj['name']}")
                    elif "data" in obj and obj["data"] == "pong":
                        await self.OnOpen(self)
            except websockets.exceptions.ConnectionClosed as e:
                print(f"Server closed the connection: {e}")
                if self.autorestart:
                    print(f"Reconnecting in {self.restartevery} seconds...")
                    asyncio.sleep(self.restartevery) 
                    self.conn =await self.connect(self.path)

    async def Subscribe(self, topic, handler, name=""):
        payload = {"action": "sub", "topic": topic, "id": self.id}
        if name:
            payload["name"] = name
            subs = BusSubscription(self, topic, name)
            self.topic_handlers[topic + ":" + name] = handler
        else:
            subs = BusSubscription(self, topic)
            self.topic_handlers[topic] = handler
        if self.conn is not None:
            await self.sendMessage(payload)
        return subs

    async def Unsubscribe(self, topic, name=""):
        payload = {"action": "unsub", "topic": topic, "id": self.id}
        if name:
            payload["name"] = name
            del self.topic_handlers[topic + ":" + name]
        else:
            del self.topic_handlers[topic]
        if topic and self.conn is not None:
            await self.sendMessage(payload)

    async def Publish(self, topic, data):
        if self.conn is not None:
            await self.sendMessage({"action": "pub", "topic": topic, "data": data, "id": self.id})
        else:
            print("Publish: Not connected to server. Please check the connection.")

    async def SendTo(self, name, data, topic=""):
        payload = {"action": "send", "name": name, "data": data,"id": self.id}
        if self.conn is not None:
            if topic:
                payload["topic"] = topic
            await self.sendMessage(payload)

    async def RemoveTopic(self, topic):
        if self.conn is not None:
            await self.sendMessage({"action": "remove", "topic": topic, "id": self.id})
            del self.topic_handlers[topic]

    async def OnClose(self, callback):
        self.on_close = callback
    
    async def sendMessage(self,obj):
        await self.conn.send(json.dumps(obj))

    def makeid(self, length):
        """helper function to generate a random string of a specified length"""
        return "".join(random.choices(string.ascii_letters + string.digits, k=length))

class BusSubscription:
    def __init__(self, bus, topic, name=""):
        self.bus = bus
        self.topic = topic
        self.name = name

    async def Unsubscribe(self):
        if self.name:
            await self.bus.conn.send(
                json.dumps(
                    {
                        "action": "unsub",
                        "topic": self.topic,
                        "name": self.name,
                        "id": self.bus.id,
                    }
                )
            )
            del self.bus.topic_handlers[self.topic]
            del self.bus.topic_handlers[self.topic + ":" + self.name]
        else:
            await self.bus.conn.send(json.dumps({"action": "unsub", "topic": self.topic, "id": self.bus.id}))
            del self.bus.topic_handlers[self.topic]
