# pip install ksbus==1.2.7
from ksbus import Bus


def OnId(data):
    print("OnId:",data)

def OnOpen(bus):
    print("connected",bus)
    bus.PublishToIDWaitRecv("browser",{
        "data":"hi from pure python"
    },lambda data:print("OnRecv:",data),lambda event_id:print("OnFail:",event_id))

if __name__ == '__main__':
    Bus({
        'id': 'py',
        'addr': 'localhost:9313',
        'OnId': OnId,
        'OnOpen':OnOpen},
        block=True)