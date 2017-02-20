// protocol
package protocol

import (
	"fmt"
	"strconv"
)

const (
	ConstHeader         = "{\"protoSign\":142857,\"msgLength\":\""
	ConstHeaderLength   = 33
	ConstSaveDataLength = 10
)

func Unpack(buffer []byte, readerChannel chan []byte) []byte {
	length := len(buffer)
	//fmt.Println(string(buffer))
	//fmt.Println(length)
	var i int
	flag := false
	for i = 0; i < length; i++ {
		if length < i+ConstHeaderLength+ConstSaveDataLength {
			break
		}

		if string(buffer[i:i+ConstHeaderLength]) == ConstHeader {
			//截取消息长度
			t, err := strconv.ParseInt(string(buffer[i+ConstHeaderLength:i+ConstHeaderLength+ConstSaveDataLength]), 0, 0)
			if err != nil {
				fmt.Println(err.Error())
			}
			messageLength := int(t)
			if i+messageLength > length {
				break
			}
			data := buffer[i : i+messageLength]
			if flag == false && i != 0 {
				return buffer[i:]
			} else {
				flag = true
			}
			readerChannel <- data
			i += messageLength - 1
		}
	}
	if i == length {
		return make([]byte, 0)
	}
	return buffer[i:]
}
