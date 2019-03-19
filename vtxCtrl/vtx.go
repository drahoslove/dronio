package vtx

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"path/filepath"
	"time"
)

// for controlling camera related stuff

const ( // meaning of ints in msgs by position
	cmdI = iota // action
	_
	_
	lenI // payload size in Bytes appended after
)

const ( // possible actions (cmdI)
	_                = 0x0001 // stream?
	takePhotoCmd     = 0x0013
	startRecordCmd   = 0x0006
	stopRecordCmd    = 0x0011
	deleteVideoCmd   = 0x0014
	downloadVideoCmd = 0x0012 // 7060
	playVideoCmd     = 0x0009 // 7060
	// listVideosCmd    = 0x0003 // 7060
	listVideosCmd = 0x0008 // 8060
	_             = 0x0101 // ?? stream ?
	_             = 0x0106 // recv videofile
	_             = 0x0010 // close stream?
)

// LeweiCmd represents data packet (not tcp, but app layer) sent or received by drones vtx controller
type LeweiCmd struct {
	// sync.RWMutex
	header  []byte // 46B => "lewei_cmd\0" + 9 × uint32 MSB (+payload)
	payload bytes.Buffer
}

// NewLeweiCmd will create new LeweiCmd with correct header initialized and given action set
func NewLeweiCmd(action uint32) LeweiCmd {
	header := make([]byte, 46)
	copy(header, "lewei_cmd\x00")
	cmd := LeweiCmd{header: header}
	cmd.headerSet(cmdI, action)
	return cmd
}

// headerSet sets value at given index in LeweiCmd header
func (c *LeweiCmd) headerSet(index uint, value uint32) {
	binary.LittleEndian.PutUint32(c.header[10+index*4:], value)
}

// headerGet will return value at given index in LeweiCmd header
func (c *LeweiCmd) headerGet(index uint) uint32 {
	return binary.LittleEndian.Uint32(c.header[10+index*4:])
}

// AddPayload appends string, byte slice, or uint32 slice to cmd
// and increase payload size accordingly
func (c *LeweiCmd) AddPayload(data interface{}) {
	if data == nil {
		return
	}
	binary.Write(&c.payload, binary.LittleEndian, data)

	addLen := func(l int) {
		l += int(c.headerGet(lenI))
		c.headerSet(lenI, uint32(l))
	}
	switch d := data.(type) {
	case string:
		addLen(len(d))
	case []byte:
		addLen(len(d))
	case []uint32:
		addLen(len(d) * 4)
	}
}

func (c *LeweiCmd) String() (str string) {
	str = string(c.header[:10])
	for part := c.header[10:]; len(part) > 0; part = part[4:] {
		str += fmt.Sprintf(" %x", part[:4])
	}
	return str
}

func newConn(port int) *net.TCPConn {
	raddr := &net.TCPAddr{IP: net.IPv4(192, 168, 0, 1), Port: port}
	laddr := &net.TCPAddr{IP: net.IPv4(192, 168, 0, 2)} // auto port
	conn, err := net.DialTCP("tcp4", laddr, raddr)
	if err != nil {
		fmt.Printf("%v", fmt.Errorf("Cant't create connection, are you on right wifi?"))
		return nil
	}
	conn.SetDeadline(time.Now().Add(time.Second * 5))
	return conn
}

func send(conn *net.TCPConn, cmd LeweiCmd) {
	conn.Write(cmd.header)
	conn.Write(cmd.payload.Bytes())
}

func recv(conn *net.TCPConn) LeweiCmd {
	cmd := NewLeweiCmd(0)
	n, err := conn.Read(cmd.header)
	if n != len(cmd.header) {
		println("not whole header", len(cmd.header), n) // correct port?
	}
	if err != nil {
		panic(err)
	}
	payloadLen := cmd.headerGet(lenI)

	cmd.payload.Grow(int(payloadLen))
	io.CopyN(&cmd.payload, conn, int64(payloadLen))
	return cmd
}

func portBytCmd(cmd uint32) int {
	switch cmd {
	case playVideoCmd, downloadVideoCmd:
		return 7060
	default:
		return 8060
	}
}

// Req will make request of type given by cmd and call callback function with response payload in byte slice
func Req(cmd uint32, payload []byte, callback func([]byte)) {
	conn := newConn(portBytCmd(cmd))
	if conn == nil {
		return
	}
	defer conn.Close()

	// send request
	req := NewLeweiCmd(cmd)
	req.AddPayload(payload)
	send(conn, req)
	println(req.String())

	// load payload:
	resp := recv(conn)
	println(resp.String())

	// check return type
	if resp.headerGet(cmdI) != cmd {
		panic("Invalid response command type")
	}
	if callback != nil {
		callback(resp.payload.Bytes())
	}
}

// func Post(cmd uint32, payload []byte) {
// 	conn := newConn(portBytCmd(cmd))
// 	if conn == nil {
// 		return
// 	}
// 	defer conn.Close()
// 	req := NewLeweiCmd(cmd)
// 	req.AddPayload(payload)
// 	send(conn, req)
// }

// TakePhoto will take photo and save to current dir
func TakePhoto() {
	Req(takePhotoCmd, nil, func(payload []byte) {
		// parse payload:
		fileSize := binary.LittleEndian.Uint32(payload[0:4])
		fileName := string(bytes.Trim(payload[3*4:3*4+100], "\x00"))
		fileContent := payload[32*4 : 32*4+fileSize]

		println(fileSize, fileName)

		// output file
		err := ioutil.WriteFile(filepath.Base(fileName), fileContent, 0777)
		if err != nil {
			panic(err)
		}
	})
}

func ListVideos() {
	Req(listVideosCmd, nil, func(payload []byte) {
		for len(payload) > 0 {
			duration := binary.LittleEndian.Uint32(payload[4:8])
			fileName := string(bytes.Trim(payload[4*4:4*4+100], "\x00"))
			println(duration, "\t", fileName)
			payload = payload[116:]
		}
	})
}

func DeleteVideo(filename string) {
	payload := make([]byte, 100)
	copy(payload, filename)
	Req(deleteVideoCmd, payload, nil)
}

func DownloadVideo(filename string) {
	payload := make([]byte, 196)
	copy(payload[4*4:], filename)
	Req(downloadVideoCmd, payload, nil)
}

// CaptureVideo will capture vide of given period of time
func CaptureVideo(duration time.Duration) {
	Req(startRecordCmd, nil, func(payload []byte) {
		println(string(payload))
	})
	time.Sleep(duration)
	Req(stopRecordCmd, nil, func(payload []byte) {
		println(string(payload))
	})
}
