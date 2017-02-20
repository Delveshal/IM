package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"protocol"
)

type User struct {
	UserName  string `json:"userName",omitempty`
	UserPWD   string `json:"userPWD",omitempty`
	UserID    string `json:"userID",omitempty`
	UserToken string `json:"userToken",omitempty`
}

type Status struct {
	ActionMsgType int8   `json:"actionMsgType",omitempty`
	ActionRslMsg  string `json:"actionRslMsg",omitempty`
	ActionRslCode int8   `json:"actionRslCode",omitempty`
}

type DataFrame struct {
	ProtoSign    int       `json:"protoSign",omitempty`
	MsgLength    string    `json:"msgLength",omitempty`
	MsgType      int8      `json:"msgType",omitempty`
	FeedbackType string    `json:"feedbackType",omitempty`
	SenderTimer  time.Time `json:"senderTimer",omitempty`
	Sender       User      `json:"sender",omitempty`
	UserList     []string  `json:"userList",omitempty`
	PayLoad      string    `json:"payLoad",omitempty`
	ActionStatus Status    `json:"actionStatus",omitempty`
	dataBuf      []byte
}

var me DataFrame
var uList map[string]string
var conList map[net.Conn]string
var clnOffLineChannel chan string
var msgChannel chan string
var clnMsgChannel chan DataFrame
var sendmsgChannel chan []byte
var flag bool

func in() {
	conList = make(map[net.Conn]string)
	uList = make(map[string]string)
	msgChannel = make(chan string)
	clnMsgChannel = make(chan DataFrame)
	clnOffLineChannel = make(chan string)
	sendmsgChannel = make(chan []byte)
	flag = false
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

func myprint(readerChannel chan []byte) {
	n := 2
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
					if flag == false && d.ActionStatus.ActionRslCode == 6 {
						if n == 0 {
							os.Exit(1)
						}
						fmt.Printf("请重新输入密码: （还有%d次机会）\n", n)
						me.Sender.UserPWD = getMsg()
						n--
						me.Marshal()
						sendmsgChannel <- me.dataBuf
					} else {
						flag = true
					}
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
func sendMessage(clnSck net.Conn) {
	//加个定时发送
	empacketstr := "{\"protoSign\":142857,\"msgLength\":\"0x00000030‬\"}"
	emptypacket := []byte(empacketstr)
	//testpacket := emptypacket[:10]
	heartbeatstime := time.NewTimer(time.Second * 30)
	//testtime := time.NewTimer(time.Second * 10)
	for {
		select {
		case data := <-sendmsgChannel:
			{
				clnSck.Write(data)
			}
		case <-heartbeatstime.C:
			{
				clnSck.Write(emptypacket)
				heartbeatstime.Reset(time.Second * 30)
			}
			/*case <-testtime.C:
			{
				clnSck.Write(testpacket)
			}*/
		}
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
	go sendMessage(clnSck)
	me.Marshal()
	sendmsgChannel <- me.dataBuf
	var dstPos, msgPos int
	for {
		if flag == false {
			continue
		}
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
		for i := 0; i < 1000; i++ {
			clnSck.Write(me.dataBuf)
		}
	}
	clnSck.Close()
}

func main() {
	in()
	srvAddr := "127.0.0.1:6666"
	fmt.Println("请输入用户名：")
	me.Sender.UserName = getMsg()
	fmt.Println("请输入密码：")
	me.Sender.UserPWD = getMsg()
	me.PayLoad = "registration"
	cln(srvAddr)
}
