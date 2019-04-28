// Package vtx is for controlling camera related stuff
//
// TCP port 7060 is for live video stream data (also for downloading/replaying captured videos)
// TCP port 8060 is for the rest - start/stop video capturing, taking pohoto, listing videos on sd card etc.
package vtx

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
	"time"
	"unsafe"
)

const (
	off = 0
	on  = 1
)

// Header of commands consists of "lewei_cmd\0" string and 9 uint32 numbers (little endian)
// only first and fourth number has known meaning so far
const (
	cmdI = iota // action
	_
	_
	lenI // payload size in Bytes appended after header
	_
	_
	_
	_
	_
)

// possible actions (cmdI)
const (
	// res/req
	keepAliveCmd    = 0x0001 // 7060
	setClockCmd     = 0x0004
	checkVideoCmd   = 0x0006
	listVideosCmd   = 0x0008
	captureVideoCmd = 0x0011
	takePhotoCmd    = 0x0013
	deleteVideoCmd  = 0x0014
	// ??
	_        = 0x0003 // 7060 // ?
	closeCmd = 0x0010 // 7060 close stream?
	// req only
	_                = 0x0002 // 7060 // start stream?
	replayVideoCmd   = 0x0009 // 7060
	downloadVideoCmd = 0x0012 // 7060
	// respo only
	_                 = 0x0101 // 7060 stream ? after 0002
	videoReplayCmd    = 0x0103 // 7060 video play after replayVideoCmd
	videoReplayEndCmd = 0x0105 // 7060 ?? replay end?
	videoDownloadCmd  = 0x0106 // recv videofile after downloadVideoCmd
)

// LeweiCmd represents data packet (app layer) sent or received by vtx of the drone
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

func newConn(port int) (*net.TCPConn, func()) {
	raddr := &net.TCPAddr{IP: net.IPv4(192, 168, 0, 1), Port: port} // IP of drone
	laddr := &net.TCPAddr{IP: getLocalIP()}                         // auto port
	conn, err := net.DialTCP("tcp4", laddr, raddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n%v\n", fmt.Errorf("Cant't create connection, are you on right wifi?"), err)
		return nil, nil
	}
	conn.SetDeadline(time.Time{})
	// conn.SetDeadline(time.Now().Add(time.Second * 50))
	closeConn := keepAlive(conn)
	return conn, closeConn
}

// KeepAlive will keep connection alive until function returned by it is called
//
// Socket will be othervise closed by the server after 5-10s if it is not written to
func keepAlive(conn *net.TCPConn) func() {
	ticker := time.NewTicker(time.Second * 2)
	stop := make(chan bool)
	go func() {
		for {
			select {
			case <-ticker.C:
				println("keepalive")
				Req(keepAliveCmd, nil, conn)
			case <-stop:
				ticker.Stop()
				conn.Close()
				return
			}
		}
	}()
	return func() {
		stop <- true
	}
}

// getLocalIP gets smallest ip in 192.168.0.* which exists in the system
func getLocalIP() net.IP {
	bestIP := net.IPv4(192, 168, 0, 255)
	addrs, _ := net.InterfaceAddrs()
	for _, addr := range addrs {
		ip := addr.(*net.IPNet).IP
		if ip.Mask(ip.DefaultMask()).Equal(net.IPv4(192, 168, 0, 0)) { // is in same subnet
			if ip[len(ip)-1] < bestIP[len(bestIP)-1] { // has lower last byte
				bestIP = ip
			}
		}
	}
	return bestIP
}

// send LeweiCmd
func send(conn *net.TCPConn, cmd LeweiCmd) error {
	_, err := conn.Write(cmd.header)
	conn.Write(cmd.payload.Bytes())
	return err
}

// recv LeweiCmd
func recv(conn *net.TCPConn) (LeweiCmd, error) {
	cmd := NewLeweiCmd(0)
	n, err := conn.Read(cmd.header)
	if n != len(cmd.header) {
		println("not whole header", len(cmd.header), n) // correct port?
	}
	if err != nil {
		return cmd, err
	}
	payloadLen := cmd.headerGet(lenI)

	cmd.payload.Grow(int(payloadLen))
	recvN := int64(0)
	for recvN < int64(payloadLen) {
		n, err := io.CopyN(&cmd.payload, conn, int64(payloadLen)-recvN)
		recvN += n
		if err != nil {
			return cmd, err
		}
	}
	return cmd, nil
}

func portByCmd(cmd uint32) int {
	switch cmd {
	case replayVideoCmd, downloadVideoCmd, keepAliveCmd:
		return 7060
	default:
		return 8060
	}
}

func byteToUint16(arr []byte) []uint16 {
	arr = arr[:]
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&arr))
	header.Len /= 2 // (16 bit = 2 bytes)
	header.Cap /= 2
	return *(*[]uint16)(unsafe.Pointer(&header))
}

func byteToUint32(arr []byte) []uint32 {
	arr = arr[:]
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&arr))
	header.Len /= 4 // (32 bit = 4 bytes)
	header.Cap /= 4
	return *(*[]uint32)(unsafe.Pointer(&header))
}

// Action combines together Req and Res functions and open/closes own connection
//
// it will make request of type given by cmd and call callback function with response payload in byte slice
func Action(cmd uint32, payload interface{}, callback func([]byte)) {
	conn, closeConn := newConn(portByCmd(cmd))
	if conn == nil {
		return
	}
	defer closeConn()
	Req(cmd, payload, conn)
	data := Res(cmd, conn)

	if callback != nil {
		callback(data)
	}
}

// Req will create and send request to TCP conn
//
// Use Action instead, if you expect response with same cmd type
func Req(cmd uint32, payload interface{}, conn *net.TCPConn) {
	// send request
	req := NewLeweiCmd(cmd)
	req.AddPayload(payload)
	send(conn, req)
}

// Res will obtain response from TCP conn (while skipping keepalive cmds)
//
// Use Action instead, if tis is response for requsest of same cmd type
func Res(cmd uint32, conn *net.TCPConn) (payload []byte) {
	// load payload:
start:
	resp, _ := recv(conn)

	// check return type
	recvCmd := resp.headerGet(cmdI)
	if recvCmd != cmd {
		if recvCmd == keepAliveCmd {
			// ignore keepAlive response and start over
			goto start
		}
		if cmd == videoReplayCmd && recvCmd == videoReplayEndCmd {
			println("video replay end??")
			return resp.payload.Bytes()
		}
		if recvCmd == 0 { // closed channel? retun empty cmd
			return []byte{}
		}
		panic(fmt.Errorf("invalid response command type; exp %v; got %v", cmd, recvCmd))
	}
	conn.SetDeadline(time.Now().Add(time.Second * 10))

	return resp.payload.Bytes()
}
