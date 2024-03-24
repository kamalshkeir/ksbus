# pip install ksbus==1.2.8
from ksbus import Bus


def OnId(data):
    print("OnId:",data)

def OnOpen(bus):
    print("connected as ",bus.Id)
    bus.PublishToIDWaitRecv("browser",{
        "data":"hi from pure python"
    },lambda data:print("OnRecv:",data),lambda event_id:print("OnFail:",event_id))

if __name__ == '__main__':
    Bus({
        'Id': 'py',
        'Address': 'localhost:9313',
        'OnId': OnId,
        'OnOpen':OnOpen},
        block=True)