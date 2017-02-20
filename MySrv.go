package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"protocol"
	"strconv"
	"strings"
	"syncmap"
	"time"
)

type User struct {
	UserName  string `json:"userName",omitempty`
	UserPWD   string `json:"userPWD",omitempty`
	UserID    string `json:"userID",omitempty`
	UserToken string `json:"userToken",omitempty`
}

type Status struct {
	ActionMsgType int    `json:"actionMsgType",omitempty`
	ActionRslMsg  string `json:"actionRslMsg",omitempty`
	ActionRslCode int    `json:"actionRslCode",omitempty`
}

type DataFrame struct {
	ProtoSign    int       `json:"protoSign",omitempty`
	MsgLength    string    `json:"msgLength",omitempty`
	MsgType      int       `json:"msgType",omitempty`
	FeedbackType string    `json:"feedbackType",omitempty`
	SenderTimer  time.Time `json:"senderTimer",omitempty`
	Sender       User      `json:"sender",omitempty`
	UserList     []string  `json:"userList",omitempty`
	PayLoad      string    `json:"payLoad",omitempty`
	ActionStatus Status    `json:"actionStatus",omitempty`
	dataBuf      []byte
}

var me DataFrame
var uList *syncmap.SyncMap
var conList *syncmap.SyncMap
var list *syncmap.SyncMap
var clnOffLineChannel chan string
var clnOnLineChannel chan string
var msgChannel chan string
var clnMsgChannel chan DataFrame
var backMsgChannel chan DataFrame

func showOnlines(count int) {
	fmt.Println("Online Number: " + strconv.Itoa(count))
}

func (d *DataFrame) Marshal() error {
	if len(d.Sender.UserName) <= 0 || len(d.Sender.UserPWD) <= 0 || len(d.PayLoad) <= 0 {
		return errors.New("用户名或者密码为空")
	}
	d.ProtoSign = 142857
	d.MsgLength = "0x12345678"
	var err error
	d.dataBuf, err = json.Marshal(d)
	if err != nil {
		return err
	}
	lenn := len(d.dataBuf)
	d.MsgLength = fmt.Sprintf("%#08x", lenn)
	d.dataBuf, err = json.Marshal(d)
	return err
}

func in() {
	conList = syncmap.New()
	uList = syncmap.New()
	list = syncmap.New()
	msgChannel = make(chan string)
	clnMsgChannel = make(chan DataFrame)
	clnOffLineChannel = make(chan string)
	clnOnLineChannel = make(chan string)
	backMsgChannel = make(chan DataFrame)
}

func setMe(name string, PWD string, msg string) {
	if name == "" {
		me.Sender.UserName = "admin"
	} else {
		me.Sender.UserName = name
	}
	if PWD == "" {
		me.Sender.UserPWD = "123456"
	} else {
		me.Sender.UserPWD = PWD
	}
	if msg == "" {
		me.PayLoad = "registration"
	} else {
		me.PayLoad = msg
	}
	//uList[me.Sender.UserName] = me.Sender.UserPWD
	uList.Set(me.Sender.UserName, me.Sender.UserPWD)
}

func getMsg() (msg string) {
	reader := bufio.NewReader(os.Stdin)
	msg, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	msg = strings.Replace(msg, "\n", "", -1)
	msg = msg[:len(msg)-1]
	return
}

func mgrRecv(name *string, readerChannel chan []byte, clnSck net.Conn) {
	empacketstr := "{\"protoSign\":142857,\"msgLength\":\"0x00000030‬\"}"
	k := time.Now()
	s, _ := time.ParseDuration("31s")
	for {
		select {
		case data := <-readerChannel:
			{ //加个判断是不是空包
				if string(data) == empacketstr {
					fmt.Println("这是个空包", *name)
					if k.Add(s).Before(time.Now()) {
						fmt.Println(*name, "丢失")
						clnOffLineChannel <- *name
					} else {
						fmt.Println("保持连接")
						k = time.Now()
					}
					continue
				}
				var d DataFrame
				err := json.Unmarshal(data, &d)
				if err != nil {
					fmt.Println(err.Error())
					continue
				}
				d.dataBuf = make([]byte, len(data))
				copy(d.dataBuf, data)
				tryPWD, ok := uList.Get(d.Sender.UserName) //uList[d.Sender.UserName]
				if ok && (tryPWD != d.Sender.UserPWD) {
					fmt.Println("密码错误")
					d.ActionStatus.ActionMsgType = 22
					d.ActionStatus.ActionRslMsg = "密码错误或者用户名已存在"
					d.ActionStatus.ActionRslCode = 6
					d.FeedbackType = "s"
					if err := d.Marshal(); err != nil {
						fmt.Println(err)
					} else {
						clnSck.Write(d.dataBuf)
					}
					continue
				}

				trySck, ok := list.Get(d.Sender.UserName) //list[d.Sender.UserName]
				if ok && trySck != clnSck {
					d.ActionStatus.ActionMsgType = 22
					d.ActionStatus.ActionRslMsg = "该帐号已经在别处登录"
					d.ActionStatus.ActionRslCode = 55
					d.FeedbackType = "s"
					if err := d.Marshal(); err != nil {
						fmt.Println(err)
					} else {
						clnSck.Write(d.dataBuf)
					}
					continue
				}

				//conList[d.Sender.UserName] = clnSck
				conList.Set(d.Sender.UserName, clnSck)
				//_, ok = list[d.Sender.UserName]
				_, ok = list.Get(d.Sender.UserName)
				//list[d.Sender.UserName] = clnSck
				list.Set(d.Sender.UserName, clnSck)
				*name = d.Sender.UserName
				if !ok {
					clnOnLineChannel <- *name
				}
				clnMsgChannel <- d
			}
		}
	}
}

func srvRecv(clnSck net.Conn) {
	tmpBuffer := make([]byte, 0)
	readerChannel := make(chan []byte)
	buf := make([]byte, 1024*1024*1024)
	name := ""
	go mgrRecv(&name, readerChannel, clnSck)
	for {
		dataLen, err := clnSck.Read(buf)
		if err != nil {
			fmt.Println(err)
			if len(name) > 0 {
				clnOffLineChannel <- name
			}
			return
		}
		tmpBuffer = protocol.Unpack(append(tmpBuffer, buf[:dataLen]...), readerChannel)
	}
}

func clnMgr() {
	for {
		select {
		case name := <-clnOffLineChannel:
			//			for k, v := range conList {
			//				if v == name {
			//					fmt.Println(v + "(" + k.RemoteAddr().String() + ")" + " offline")
			//					delete(conList, k)
			//					delete(list, v)
			//					k.Close()
			//					showOnlines(conList)
			//					break
			//				}
			//			}
			//k := conList[name]
			k, _ := conList.Get(name)
			b, ok := k.(net.Conn)
			if ok {
				fmt.Println(name + "(" + b.RemoteAddr().String() + ")" + " offline")
				b.Close()
			}
			//delete(conList, name)
			conList.Delete(name)
			//delete(list, name)
			list.Delete(name)
			showOnlines(conList.Size())
			break
		case name := <-clnOnLineChannel:
			//			for k, v := range conList {
			//				if v == name {
			//					fmt.Println(v + "(" + k.RemoteAddr().String() + ")" + " oline")
			//					showOnlines(conList)
			//					break
			//				}
			//			}
			k, _ := conList.Get(name)
			b, ok := k.(net.Conn)
			if ok {
				fmt.Println(name + "(" + b.RemoteAddr().String() + ")" + " oline")
			}
			showOnlines(conList.Size())
			break
		case transMsg := <-clnMsgChannel:
			if len(transMsg.Sender.UserName) <= 0 || len(transMsg.Sender.UserPWD) <= 0 {
				fmt.Println("用户名为空在或密码为空")
				continue
			}
			_, ok := uList.Get(transMsg.Sender.UserName) //uList[transMsg.Sender.UserName]
			if !ok {
				fmt.Println("新用户注册")
				uList.Set(transMsg.Sender.UserName, transMsg.Sender.UserPWD) //
				transMsg.ActionStatus.ActionMsgType = 22
				transMsg.ActionStatus.ActionRslMsg = "新用户注册成功"
				transMsg.ActionStatus.ActionRslCode = 0
				transMsg.FeedbackType = "s"
				if err := transMsg.Marshal(); err != nil {
					fmt.Println(err)
				} else {
					//list[transMsg.Sender.UserName].Write(transMsg.dataBuf)
					k, _ := list.Get(transMsg.Sender.UserName)
					b, ok := k.(net.Conn)
					if ok {
						b.Write(transMsg.dataBuf)
					}
				}
				continue
			}
			failMsg := "Fail send message to"
			for i := range transMsg.UserList {
				dstUser, _ := list.Get(transMsg.UserList[i]) //list[transMsg.UserList[i]]
				if dstUser == nil {
					failMsg += " " + transMsg.UserList[i]
					continue
				}
				if transMsg.dataBuf == nil || len(transMsg.dataBuf) <= 0 {
					fmt.Println("transMsg.dataBuf is nil/empty")
					continue
				}
				b, ok := dstUser.(net.Conn)
				var err error
				if ok {
					_, err = b.Write(transMsg.dataBuf)
				}

				if err != nil {
					fmt.Println(err)
					failMsg += " " + transMsg.UserList[i]
					clnOffLineChannel <- transMsg.UserList[i]
					continue
				}
			}
			if len(failMsg) == len("Fail send message to") && failMsg == "Fail send message to" {
				if transMsg.UserList == nil {
					transMsg.ActionStatus.ActionRslMsg = transMsg.Sender.UserName + " 登陆成功"
				} else {
					transMsg.ActionStatus.ActionRslMsg = "发送成功"
				}
				transMsg.ActionStatus.ActionRslCode = 0
			} else {
				transMsg.ActionStatus.ActionRslMsg = failMsg
				transMsg.ActionStatus.ActionRslCode = 1
			}
			transMsg.ActionStatus.ActionMsgType = 22
			transMsg.FeedbackType = "s"
			if err := transMsg.Marshal(); err != nil {
				fmt.Println(err)
			} else {
				k, _ := list.Get(transMsg.Sender.UserName) //list[transMsg.Sender.UserName].Write(transMsg.dataBuf)
				b, ok := k.(net.Conn)
				if ok {
					b.Write(transMsg.dataBuf)
				}
			}
		}
	}
}

func myprint(readerChannel chan []byte) {
	for {
		select {
		case data := <-readerChannel:
			{
				var d DataFrame
				if err := json.Unmarshal(data, &d); err != nil {
					fmt.Println(err.Error())
					continue
				}
				if d.ActionStatus.ActionMsgType == 22 {
					fmt.Println(d.ActionStatus.ActionRslMsg)
					continue
				}
				fmt.Println(d.PayLoad)
				fmt.Println(" from ", d.Sender.UserName)
			}
		}
	}
}

func clnRecv(clnSck net.Conn) {
	const MAX_FRAME_SIZE = 1024 * 4
	buf := make([]byte, MAX_FRAME_SIZE)
	tmpBuffer := make([]byte, 0)
	readerChannel := make(chan []byte)
	go myprint(readerChannel)
	//frameBuf := make([]byte, MAX_FRAME_SIZE+MAX_FRAME_SIZE/2)
	for {
		dataLen, err := clnSck.Read(buf)
		if err != nil {
			log.Fatalln(err.Error())
			return
		}
		tmpBuffer = protocol.Unpack(append(tmpBuffer, buf[:dataLen]...), readerChannel)
	}
}

func cln(srvAddr string) {
	srv, err := net.ResolveTCPAddr("tcp", srvAddr)
	if err != nil {
		fmt.Println(err)
		return
	}
	clnSck, err := net.DialTCP("tcp", nil, srv)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer clnSck.Close()
	go clnRecv(clnSck)
	me.Marshal()
	clnSck.Write(me.dataBuf)
	var dstPos, msgPos int
	for {
		msg := strings.Replace(
			strings.Replace(
				strings.Replace(
					strings.Replace(
						strings.Replace(getMsg(), "　", " ", -1),
						"，", ",", -1),
					"＠", "@", -1),
				", ", ",", -1),
			" ,", ",", -1)

		dstPos = strings.Index(msg, "@")

		if dstPos == 0 {
			msgPos = strings.IndexAny(msg, " ")
			if msgPos <= 0 {
				fmt.Println("empty msg")
				continue
			}
			me.UserList = strings.Split(msg[1:msgPos], ",")
			me.PayLoad = msg[msgPos:]
		} else {
			me.PayLoad = msg
		}
		if len(me.PayLoad) <= 0 {
			fmt.Println("empty msg")
			continue
		}

		if err := me.Marshal(); err != nil {
			fmt.Println(err.Error())
			continue
		}
		clnSck.Write(me.dataBuf)
	}
	clnSck.Close()
}

func main() {
	in()
	setMe("admin", "123", "registration")
	srvSck, err := net.Listen("tcp", ":6666")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer srvSck.Close()
	go clnMgr()
	go cln("127.0.0.1:6666")
	for {
		clnSck, err := srvSck.Accept()
		if err != nil {
			fmt.Println(err)
			return
		}
		go srvRecv(clnSck)
	}
}
