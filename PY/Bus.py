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
    def __init__(self, addr,block=False, path="/ws/bus", secure=False, onOpen = lambda bus:None):
        self.scheme = "ws://"
        if secure:
            self.scheme = "wss://"
        self.path = self.scheme + addr + path
        self.conn = None
        self.topic_handlers = {}
        self.autorestart = False
        self.restartevery = 5
        if onOpen:
            self.on_open = onOpen                           
        else:
            print("no onOpen callback was given in bus constructor")
            return
        self.OnData = None
        self.on_close = lambda: None
        self.id = self.makeid(8)
        try:
            if block :
                asyncio.get_event_loop().run_until_complete(self.connect(self.path))
            else:
                asyncio.create_task(self.connect(self.path))
        except Exception as e:
            print(e)
    
    async def connect(self, path):
        try:
            self.conn = await websockets.connect(path)
            await self.sendMessage({"action": "ping", "id": self.id})
            async for message in self.conn:
                obj = json.loads(message)
                if self.OnData is not None:
                    self.OnData(obj)
                if "event_id" in obj:
                    self.Publish(obj["event_id"],{
                        "success":"got the event",
                        "id":self.id
                    })
                if "topic" in obj:
                    if obj["topic"] in self.topic_handlers:
                        subs = BusSubscription(self, obj["topic"])
                        self.topic_handlers[obj["topic"]](obj, subs)
                    else:
                        print(
                            f"topicHandler not found for: {obj['topic']}")
                elif "data" in obj and obj["data"] == "pong":
                    if self.on_open is None:
                        print("no onOpen callback was given in bus constructor")
                        return
                    else:
                        self.on_open(self)
        except Exception as e:
            print(f"Server closed the connection: {e}")
            if self.autorestart:
                loop = True
                while loop:
                    print(f"Reconnecting in {self.restartevery} seconds...")
                    await asyncio.sleep(self.restartevery)
                    await self.connect(self.path)

    def Subscribe(self, topic, handler):
        payload = {"action": "sub", "topic": topic, "id": self.id}
        subs = BusSubscription(self, topic)
        self.topic_handlers[topic] = handler
            
        if self.conn is not None:
            asyncio.create_task(self.sendMessage(payload))
        return subs

    def Unsubscribe(self, topic):
        payload = {"action": "unsub", "topic": topic, "id": self.id}
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
            asyncio.create_task(self.sendMessage(
                {"action": "pub", "topic": topic, "data": data, "id": self.id}))
        else:
            print("Publish: Not connected to server. Please check the connection.")

    def PublishToID(self, topic, data):
        if self.conn is not None:
            asyncio.create_task(self.sendMessage(
                {"action": "pub_id", "id": self.id, "data": data, "from": self.id}))
        else:
            print("PublishToID: Not connected to server. Please check the connection.")

    def RemoveTopic(self, topic):
        if self.conn is not None:
            asyncio.create_task(self.sendMessage(
                {"action": "remove", "topic": topic, "id": self.id}))
            del self.topic_handlers[topic]

    def OnClose(self, callback):
        self.on_close = callback

    def makeid(self, length):
        """helper function to generate a random string of a specified length"""
        return "".join(random.choices(string.ascii_letters + string.digits, k=length))


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
        asyncio.create_task(self.sendMessage({"action": "unsub", "topic": self.topic, "id": self.bus.id}))
        del self.bus.topic_handlers[self.topic]
            