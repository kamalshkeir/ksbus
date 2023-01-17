# Ksbus Python client

```py
from ksbus import Bus


# pythonTopicHandler handle topic 'python'
async def pythonTopicHandler(data,subs):
    print("recv on topic python:",data)
    # Unsubscribe
    #await subs.Unsubscribe()

# onOpen callback that let you know when connection is ready, it take the bus as param
async def onOpen(bus):
    print("connected")
    # Publish publish to topic
    await bus.Publish("top",{
        "data":"hello from python"
    })
    # Subscribe, it also return the subscription
    await bus.Subscribe("python",pythonTopicHandler)
    # SendTo publish to named topic
    await bus.SendTo("top:srv",{
        "data":"hello again from python"
    })
    

if __name__ == "__main__":
    Bus("localhost:9313",onOpen=onOpen)
```