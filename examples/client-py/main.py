# pip install ksbus==1.2.9
from ksbus import Bus


def OnId(data):
    print("OnId:",data)

def handlerPY(data,subscription):
    print("got on py topic:",data)

def OnOpen(bus):
    print("connected as ",bus.Id)
    bus.Subscribe("py",handlerPY)
    bus.Publish("server1",{
        "msg":"pooling"
    })
    # bus.PublishToIDWaitRecv("master",{
    #     "data":"hi from pure python"
    # },lambda data:print("OnRecv:",data),lambda event_id,id:print("OnFail:",event_id,id))
    

if __name__ == '__main__':
    Bus({
        'Id': 'python-client',
        'Address': 'localhost:9313',
        'OnId': OnId,
        'OnOpen':OnOpen},
        block=True)