class Bus {
    /**
     * Bus can be initialized without any param 'let bus = new Bus()'
     * @param {object} options "default: {...}"
     * @param {string} options.id "default: uuid"
     * @param {string} options.addr "default: window.location.host"
     * @param {string} options.path "default: /ws/bus"
     * @param {boolean} options.secure "default: false"
     * @param {boolean} options.autorestart "default: false"
     * @param {number} options.restartevery "default: 10"
     */
    constructor(options) {
        if (options === undefined) {
            options = {}
        }
        this.addr = options.addr || window.location.host;
        this.path = options.path || "/ws/bus";
        this.scheme = options.scheme || "ws://";
        this.fullAddress = this.scheme + this.addr + this.path;
        this.TopicHandlers = {};
        this.autorestart = options.autorestart || false;
        this.restartevery = options.restartevery || 10;
        this.secure = options.secure || false;
        if (this.secure) {
            this.scheme = "wss://"
        }
        this.OnOpen = () => { };
        this.OnClose = () => { };
        this.OnDataWs = (data, ws) => { };
        this.OnId = (data) => { };
        this.id = options.id || this.makeid();
        this.conn = this.connect(this.fullAddress, this.callback);
    }

    connect(path, callbackOnData) {
        let $this = this;
        $this.conn = new WebSocket(path);
        $this.conn.binaryType = 'arraybuffer';
        $this.conn.onopen = (e) => {
            console.log("Bus Connected");
            $this.conn.send(JSON.stringify({
                "action": "ping",
                "from": $this.id
            }));
            $this.TopicHandlers = {};
            $this.OnOpen();
        };

        $this.conn.onmessage = (e) => {
            let obj = JSON.parse(e.data);
            $this.subscription = {};
            $this.OnDataWs(obj, $this.conn);
            if ($this.OnId !== undefined && obj.to_id === $this.id) {
                delete obj.to_id
                $this.OnId(obj);
            }
            if (obj.event_id !== undefined) {
                $this.Publish(obj.event_id, {
                    "ok": "done",
                    "from": $this.id
                })
            }
            if (obj.topic !== undefined) {
                // on publish
                if ($this.TopicHandlers[obj.topic] !== undefined) {
                    let subs = new busSubscription($this, obj.topic);
                    $this.TopicHandlers[obj.topic](obj, subs);
                    return;
                } else {
                    console.log("topicHandler not found for topic:", obj.topic);
                }
            }
        };

        $this.conn.onclose = (e) => {
            $this.OnClose();
            if ($this.autorestart) {
                console.log('Socket is closed. Reconnect will be attempted in ' + this.restartevery + ' second.', e.reason);
                setTimeout(function () {
                    $this.conn = $this.connect(path, callbackOnData);
                }, this.restartevery * 1000);
            } else {
                console.log('Socket is closed:', e.reason);
            }
        };

        $this.conn.onerror = (err) => {
            console.log('Socket encountered error: ', err.message, 'Closing socket');
            $this.conn.close();
        };
        return $this.conn;
    }

    /**
     * Subscribe subscribe to a topic
     * @param {string} topic 
     * @param {function handler(data: string,subscription: busSubscription) {}} handler 
     */
    Subscribe(topic, handler) {
        this.conn.send(JSON.stringify({
            "action": "sub",
            "topic": topic,
            "from": this.id
        }));
        let subs = new busSubscription(this, topic);
        this.TopicHandlers[topic] = handler;
        return subs;
    }

    /**
     * Unsubscribe unsubscribe from topic
     * @param {string} topic 
     */
    Unsubscribe(topic) {
        let data = {
            "action": "unsub",
            "topic": topic,
            "from": this.id
        }
        this.conn.send(JSON.stringify(data));
        if (this.TopicHandlers !== undefined) {
            delete this.TopicHandlers[topic];
        }
    }

    /**
     * Publish publish to topic
     * @param {string} topic 
     * @param {object} data 
     */
    Publish(topic, data) {
        this.conn.send(JSON.stringify({
            "action": "pub",
            "topic": topic,
            "data": data,
            "from": this.id
        }));
    }

    /**
     * PublishWaitRecv publish to topic and exec onRecv fn when recv data
     * @param {string} topic 
     * @param {object} data 
     * @param {function} onRecv 
     * @param {function} onExpire 
     */
    PublishWaitRecv(topic, data, onRecv, onExpire) {
        data.from = this.id;
        data.topic = topic;
        let eventId = this.makeid();
        data.event_id = eventId;
        let done = false;

        this.Subscribe(eventId, (data, ch) => {
            done = true;
            if (onRecv) {
                onRecv(data);
                ch.Unsubscribe()
            }
        });
        this.Publish(topic, data);
        let timer = setTimeout(() => {
            clearTimeout(timer);
            if (!done) {
                if (onExpire) {
                    onExpire(eventId, topic);
                }
            }
        }, 500);
    }

    /**
     * PublishToIDWaitRecv publish to topic and exec onRecv fn when recv data
     * @param {string} topic 
     * @param {object} data 
     * @param {function} onRecv 
     * @param {function} onExpire 
     */
    PublishToIDWaitRecv(id, data, onRecv, onExpire) {
        data.from = this.id;
        data.id = id;
        let eventId = this.makeid();
        data.event_id = eventId;
        let done = false;

        this.Subscribe(eventId, (data, ch) => {
            done = true;
            if (onRecv) {
                onRecv(data);
                ch.Unsubscribe();
            }
        });
        this.PublishToID(id, data);
        let timer = setTimeout(() => {
            clearTimeout(timer);
            if (!done) {
                if (onExpire) {
                    onExpire(eventId, id);
                }
            }
        }, 500);
    }



    /**
     * PublishToServer publish to a server using addr like localhost:4444 or domain name https
     * @param {string} addr 
     * @param {object} data 
     * @param {boolean} secure 
     */
    PublishToServer(addr, data, secure) {
        this.conn.send(JSON.stringify({
            "action": "pub_server",
            "addr": addr,
            "data": data,
            "secure": secure,
            "from": this.id
        }));
    }

    /**
    * PublishToID publish to client or server id
    * @param {string} id 
    * @param {object} data 
    */
    PublishToID(id, data) {
        this.conn.send(JSON.stringify({
            "action": "pub_id",
            "id": id,
            "data": data,
            "from": this.id
        }));
    }

    /**
     * RemoveTopic remove a topic completely from the server bus
     * @param {string} topic 
     * @returns 
     */
    RemoveTopic(topic) {
        if (topic !== "") {
            this.conn.send(JSON.stringify({
                "action": "remove_topic",
                "topic": topic,
                "from": this.id
            }));
            return
        } else {
            console.error("RemoveTopic error: " + topic + " cannot be empty")
        }
    }

    makeid() {
        return "10000000-1000-4000-8000-100000000000".replace(/[018]/g, c =>
            (c ^ crypto.getRandomValues(new Uint8Array(1))[0] & 15 >> c / 4).toString(16)
        );
    }
}

/**
 * busSubscription is a class with one method allowing unsubscribing from a topic
 */
class busSubscription {
    constructor(cl, topic) {
        this.topic = topic;
        this.parent = cl;
    }
    /**
     * Unsubscribe take no params, unsubscribe from the topic
     */
    Unsubscribe() {
        this.parent.Unsubscribe(this.topic);
    }
}