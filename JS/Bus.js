class Bus {
    /**
     * Bus can be initialized without any param 'let bus = new Bus()'
     * @param {string} addr "default: window.location.host"
     * @param {string} path "default: /ws/bus"
     * @param {boolean} secure "default: false"
     */
    constructor(options) {
        this.addr=options.addr || window.location.host;
        this.path=options.path || "/ws/bus";
        this.scheme=options.scheme || "ws://";
        this.secure=options.secure || false;
        if (this.secure) {
            this.scheme="wss://"
        }
        this.fullAddress=this.scheme+this.addr+this.path;
        this.TopicHandlers={};
        this.autorestart=false;
        this.restartevery=10;
        this.OnOpen=() =>{};
        this.OnClose=() =>{};
        this.OnData=(data) =>{};
        this.id=this.makeid(8);
        this.conn=this.connect(this.fullAddress,this.callback);
    }

    connect(path,callbackOnData) {
        let $this=this;
        $this.conn = new WebSocket(path);
        $this.conn.binaryType = 'arraybuffer';
        $this.conn.onopen = (e) => {
            console.log("Bus Connected");
            $this.conn.send(JSON.stringify({
                "action":"ping",
                "id":$this.id
            }));
            $this.TopicHandlers={};
            $this.OnOpen();
        };

        $this.conn.onmessage = (e) =>  {
            let obj = JSON.parse(e.data);
            $this.subscription={};
            $this.OnData(obj);
            if (obj.event_id !== undefined) {
                $this.Publish(obj.event_id,{
                    "success":"got the event",
                    "id":$this.id
                })
            }
            if (obj.topic !== undefined) {
                // on publish
                if($this.TopicHandlers[obj.topic] !== undefined) {
                    let subs;
                    if (obj.name !== undefined) {
                        subs = new busSubscription($this,obj.topic,obj.name);
                    } else {
                        subs = new busSubscription($this,obj.topic);
                    }
                    $this.TopicHandlers[obj.topic](obj,subs);          
                    return;
                } else if(obj.name !== undefined && $this.TopicHandlers[obj.topic+":"+obj.name] !== undefined) {
                    let subs;
                    if (obj.name !== undefined) {
                        subs = new busSubscription($this,"",obj.topic+":"+obj.name);
                    } else {
                        subs = new busSubscription($this,obj.topic);
                    }
                    $this.TopicHandlers[obj.topic+":"+obj.name](obj,subs);          
                    return;
                } else {
                    console.log("topicHandler not found for topic:",obj.topic,obj.name,$this.TopicHandlers);
                }
            } else if (obj.name !== undefined) {
                // on sendTo
                let top;
                let nam;
                if (obj.name.includes(":")) {
                    let sp = obj.name.split(":");
                    top = sp[0];
                    nam = sp.slice(1).join(":");
                } else {
                    if (obj.topic) {
                        top=obj.topic
                    }
                    nam=obj.name
                }
                if($this.TopicHandlers[nam] !== undefined) {
                    let subs= new busSubscription($this,top,nam);
                    $this.TopicHandlers[obj.name](obj,subs)               
                    return;
                } else if($this.TopicHandlers[top+":"+nam] !== undefined) {
                    let subs= new busSubscription($this,top,nam);
                    $this.TopicHandlers[top+":"+nam](obj,subs)               
                    return;
                } else {
                    console.log("topicHandler not found for name:",obj.name);
                }
            } 
        };

        $this.conn.onclose =  (e) => {
            $this.OnClose();
            if ($this.autorestart) {
                console.log('Socket is closed. Reconnect will be attempted in '+this.restartevery+' second.', e.reason);
                setTimeout( function() {
                    $this.conn=$this.connect(path,callbackOnData);
                }, this.restartevery*1000);
            } else {
                console.log('Socket is closed:', e.reason);
            }
        };

        $this.conn.onerror =  (err) => {
            console.log('Socket encountered error: ', err.message, 'Closing socket');
            $this.conn.close();
        };
        return $this.conn;
    }

    /**
     * Subscribe subscribe to a topic with optional name
     * @param {string} topic 
     * @param {function handler(data: string,subscription: busSubscription) {}} handler 
     * @param {string} name 
     * @returns 
     */
    Subscribe(topic,handler,name="") {
        if (name !== "") {
            this.conn.send(JSON.stringify({
                "action":"sub",
                "topic":topic,
                "name":name,
                "id":this.id
            }));
            let subs = new busSubscription(this,topic,name);
            this.TopicHandlers[topic]=handler;
            this.TopicHandlers[topic+":"+name]=handler;
            return subs;
        }
        this.conn.send(JSON.stringify({
            "action":"sub",
            "topic":topic,
            "id":this.id
        }));
        let subs = new busSubscription(this,topic,name);
        this.TopicHandlers[topic]=handler;
        return subs;
    }

    /**
     * Unsubscribe unsubscribe from topic, name is optional
     * @param {string} topic 
     * @param {string} name 
     */
    Unsubscribe(topic,name="") {
        let data = {
            "action":"unsub",
            "topic":topic,
            "id":this.id
        }
        if (name !== "") {
            data.name=name;
        }
        this.conn.send(JSON.stringify(data));
        if (this.TopicHandlers !== undefined) {
            delete this.TopicHandlers[topic];
        } 
        if (name !== "" && this.TopicHandlers !== undefined) {
            delete this.TopicHandlers[topic+":"+name];
        }
    }

    /**
     * Publish publish to topic
     * @param {string} topic 
     * @param {object} data 
     */
    Publish(topic,data) {
        this.conn.send(JSON.stringify({
            "action":"pub",
            "topic":topic,
            "data":data,
            "id":this.id
        }));
    }

    /**
     * SendToNamed send to a named topic name must be like 'client:go'
     * @param {string} name 
     * @param {object} data 
     * @param {string} topic 
     */
    SendToNamed(name,data,topic="") {
        let toSenddata = {
            "action":"send",
            "name":name,
            "data":data,
            "id":this.id
        }
        if(topic !== "") {
            toSenddata.topic=topic
        }
        this.conn.send(JSON.stringify(toSenddata)); 
    } 
    
    /**
     * RemoveTopic remove a topic completely from the server bus
     * @param {string} topic 
     * @returns 
     */
    RemoveTopic(topic) {
        if(topic !== "") {
            this.conn.send(JSON.stringify({
                "action":"remove_topic",
                "topic":topic,
                "id":this.id
            })); 
            return
        } else {
            console.error("RemoveTopic error: "+topic+" cannot be empty")
        }
    } 

    makeid(length) {
        var result           = '';
        var characters       = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
        var charactersLength = characters.length;
        for ( var i = 0; i < length; i++ ) {
            result += characters.charAt(Math.floor(Math.random() * charactersLength));
        }
        return result;
    }
}

/**
 * busSubscription is a class with one method allowing unsubscribing from a topic without the need of arguments like topic name
 */
class busSubscription {
    constructor(cl,topic,name="") {
        this.topic=topic;
        this.name=name;
        this.parent=cl;
    }
    /**
     * Unsubscribe take no params, unsubscribe from the topic
     */
    Unsubscribe() {
        this.parent.Unsubscribe(this.topic,this.name);
    }
}