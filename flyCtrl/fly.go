// Module fly implements functionality of remote controller for visuo drone family (xs809s, xs809, xs809w, xs809h, xs809hw,...)
package fly

import (
	"fmt"
	"gobot.io/x/gobot"
	"log"
	"net"
	"sync"
	"time"
)

// Named indexes to cmd data array
const (
	_ = iota
	rollByte
	pitchByte
	throttleByte
	yawByte
	flagsByte
	crcByte
	_
)

// Meaning of bites in bitflags byte of cmd
const (
	takeOffFlag = 1 << iota
	landFlag
	stopFlag
	flipFlag
	compassFlag
	photoFlag // does not work for fpv model - it only blinks
	videoFlag // does not work for fpv model - it only blinks
	gyroFlag
)

type Cmd struct {
	sync.RWMutex
	data []byte
}

func NewCmd() Cmd {
	return Cmd{
		//              roll        throttle      bitflags       const
		//       const    \   pitch     |    yaw      /    crc    /
		//           \     \     \      |     |      /     /     /
		data: []byte{0x66, 0x80, 0x80, 0x80, 0x80, 0x00, 0x00, 0x99},
	}
}

func (c *Cmd) update(f func([]byte)) {
	c.Lock()
	f(c.data)
	c.data[crcByte] = 0
	c.data[crcByte] = crc(c.data)
	c.Unlock()
}

func (c *Cmd) isValid() bool {
	return len(c.data) == 8 && c.data[0] == 0x66 && c.data[7] == 0x99 && crc(c.data) == 0
}

func (c *Cmd) setFlag(flag byte) {
	c.update(func(data []byte) {
		data[flagsByte] |= flag
	})
}

func (c *Cmd) clearFlag(flag byte) {
	c.update(func(data []byte) {
		data[flagsByte] &^= flag
	})
}

func (c *Cmd) tempSetFlag(flag byte, duration time.Duration) {
	c.setFlag(flag)
	time.AfterFunc(duration, func() {
		c.clearFlag(flag)
	})
}

func (c *Cmd) String() (str string) {
	for _, b := range c.data {
		str += fmt.Sprintf("%02x ", b)
	}
	return
}

type Driver struct {
	sync.Mutex
	name    string
	cmd     Cmd
	stop    chan bool
	enabled bool
	udpaddr *net.UDPAddr
	laddr   *net.UDPAddr
	err     error
	onError func(error)
}

// NewDriver will create new Driver instance
//
// Optional destination and source UDP addresses might be passed as first and second argument
// Othervise 192.168.0.1:50000 is used as destination
// and automaticly choosen local system adress as source
func NewDriver(address ...string) *Driver {
	dest := "192.168.0.1:50000"
	src := "" // any
	if len(address) > 0 {
		dest = address[0]
	}
	if len(address) > 1 {
		src = address[1]
	}
	udpaddr, err := net.ResolveUDPAddr("udp4", dest)
	if err != nil {
		panic(err)
	}
	srcaddr, err := net.ResolveUDPAddr("udp4", src)
	if err != nil {
		panic(err)
	}
	return &Driver{
		name:    gobot.DefaultName("Drone"),
		cmd:     NewCmd(),
		stop:    make(chan bool),
		udpaddr: udpaddr,
		laddr:   srcaddr,
	}
}

// Name return name of the driver instance
func (d *Driver) Name() string {
	return d.name
}

// Name will set the name of the driver instance
func (d *Driver) SetName(name string) {
	d.name = name
}

func (d *Driver) Connection() gobot.Connection {
	return nil
}

// Start will start transmitting loop
func (d *Driver) Start() error {
	d.Lock()
	defer d.Unlock()
	if !d.enabled {
		d.enabled = true
		d.radioLoop()
	}
	return d.err
}

// Halt will end transmitting loop
func (d *Driver) Halt() error {
	d.Lock()
	defer d.Unlock()
	if d.enabled {
		d.stop <- true
		d.enabled = false
	}
	return d.err
}

// Set function wchich will be called when error occurs in redioLoop
func (d *Driver) OnError(callback func(err error)) {
	d.onError = callback
}

func (d *Driver) radioLoop() {

	// create connection
	conn, err := net.DialUDP("udp4", d.laddr, d.udpaddr)
	if err != nil {
		d.err = err
		d.onError(err)
		d.enabled = false
		return
	}

	go func() {
		log.Println("radio start")
		defer log.Println("radio end")
		// loop
		ticker := time.NewTicker(time.Second / 50)
		defer ticker.Stop()
		defer conn.Close()
		for now := range ticker.C {
			_ = now
			d.cmd.RLock()
			_, err := conn.Write(d.cmd.data)
			d.cmd.RUnlock()
			if err != nil {
				d.err = err
				d.onError(err)
			}
			select {
			case <-d.stop:
				d.err = nil
				return
			default:
			}
		}
	}()

}

// Reset cmd to default state
func (d *Driver) Default() {
	d.cmd.update(func(data []byte) {
		data[1] = normalize(0)
		data[2] = normalize(0)
		data[3] = normalize(0)
		data[4] = normalize(0)
		data[5] = 0
	})
}

/* Stick controll commands */

// Command drone to fly according to sticks position
//
//                      -1.0 … +1.0
// up       (throttle)    ↓ … ↑
// rotate   (yaw)         ↶ … ↷
// forwards (pitch)       ▼ … ▲
// sideways (roll)        ◀ … ▶
//
// This does not change flags byte.
func (d *Driver) Sticks(up, rotate, forwards, sideways float64) {
	d.cmd.update(func(data []byte) {
		data[rollByte] = normalize(sideways)
		data[pitchByte] = normalize(forwards)
		data[throttleByte] = normalize(up)
		data[yawByte] = normalize(rotate)
	})
}

// Command drone to stop movement and hover in place if it is in air.
// (It sets sticks to rest positions.)
func (d *Driver) Hover() {
	d.cmd.update(func(d []byte) {
		d[rollByte] = normalize(0)
		d[pitchByte] = normalize(0)
		d[throttleByte] = normalize(0)
		d[yawByte] = normalize(0)
	})
}

// Up makes the drone gain altitude.
// speed can be a value from `0` to `100`.
func (d *Driver) Up(speed int) {
	d.cmd.update(func(d []byte) { d[throttleByte] = normalize(float64(speed) / +100) })
}

// Down makes the drone reduce altitude.
// speed can be a value from `0` to `100`.
func (d *Driver) Down(speed int) {
	d.cmd.update(func(d []byte) { d[throttleByte] = normalize(float64(speed) / -100) })
}

// Right causes the drone to bank to the right, controls the roll.
// speed can be a value from `0` to `100`.
func (d *Driver) Right(speed int) {
	d.cmd.update(func(d []byte) { d[rollByte] = normalize(float64(speed) / +100) })
}

// Left causes the drone to bank to the left, controls the roll.
// speed can be a value from `0` to `100`.
func (d *Driver) Left(speed int) {
	d.cmd.update(func(d []byte) { d[rollByte] = normalize(float64(speed) / -100) })
}

// Forward causes the drone go forward, controls the pitch.
// speed can be a value from `0` to `100`.
func (d *Driver) Forward(speed int) {
	d.cmd.update(func(d []byte) { d[pitchByte] = normalize(float64(speed) / +100) })
}

// Backward causes the drone go forward, controls the pitch.
// speed can be a value from `0` to `100`.
func (d *Driver) Backward(speed int) {
	d.cmd.update(func(d []byte) { d[pitchByte] = normalize(float64(speed) / -100) })
}

// Clockwise tells drone to rotate in a clockwise direction.
// Pass in an int from 0-100.
func (d *Driver) Clockwise(speed int) {
	d.cmd.update(func(d []byte) { d[yawByte] = normalize(float64(speed) / -100) })
}

// Clockwise tells drone to rotate in a clockwise direction.
// Pass in an int from 0-100.
func (d *Driver) CounterClockwise(speed int) {
	d.cmd.update(func(d []byte) { d[yawByte] = normalize(float64(speed) / +100) })
}

/* Action commands */

// TakeOff commands drone to take off
func (d *Driver) TakeOff() {
	d.cmd.tempSetFlag(takeOffFlag, time.Second)
}

// Land commands drone to land
func (d *Driver) Land() {
	d.cmd.tempSetFlag(landFlag, time.Second)
}

// Stop commands drone to stop rotors (emergency button)
func (d *Driver) Stop() {
	d.cmd.tempSetFlag(stopFlag, time.Second)
}

// Calibrate commands drone to calibrate gyroscop
func (d *Driver) Calibrate() {
	d.cmd.tempSetFlag(gyroFlag, time.Second)
}

// CompassOn commands drone to enter compass mode
func (d *Driver) CompassOn() {
	d.cmd.setFlag(compassFlag)
}

// CompassOff commands drone to leave compass mode
func (d *Driver) CompassOff() {
	d.cmd.clearFlag(compassFlag)
}

// Flip commands drone to prepare for flip
// Making movement in some direction will cause flip in that direction.
// If drone does not make beep sound, it does not have enough power to make a flip.
func (d *Driver) Flip() {
	d.cmd.tempSetFlag(flipFlag, time.Second)
}

// TakePhoto button
// This will not work for most models - use vtx controller instead
func (d *Driver) TakePhoto() {
	d.cmd.tempSetFlag(photoFlag, time.Second)
}

// CaptureVideo button
// This will not work for most models - use vtx controller instead
func (d *Driver) CaptureVideo() {
	d.cmd.tempSetFlag(videoFlag, time.Second)
}

// BackFlip commands drone to do a backflip
func (d *Driver) BackFlip() {
	d.Flip()
	d.Backward(100)
	time.AfterFunc(time.Second, d.Hover)
}

// FrontFlip commands drone to do a frontflip
func (d *Driver) FrontFlip() {
	d.Flip()
	d.Forward(100)
	time.AfterFunc(time.Second, d.Hover)
}

// LeftFlip commands drone to do a flip to the left
func (d *Driver) LeftFlip() {
	d.Flip()
	d.Left(100)
	time.AfterFunc(time.Second, d.Hover)
}

// RightFlip commands drone to do a flip to the right
func (d *Driver) RightFlip() {
	d.Flip()
	d.Right(100)
	time.AfterFunc(time.Second, d.Hover)
}

// Convert float to byte like this
//
// -1. => 0x01
//  0. => 0x80
// +1. => 0xff
func normalize(val float64) byte {
	if val > +1 {
		val = +1
	}
	if val < -1 {
		val = -1
	}
	return byte(128 + val*127)
}

// cyclic redundancy check (polynom = 1)
//            crc
//    --[1][1][1][1][1][1][1][1] <-- xor <-- bytes
//   |________________________________^
func crc(bytes []byte) byte {
	crc := ^byte(0)
	for _, byt := range bytes {
		for i := uint(7); i < ^uint(0); i-- {
			crc = (crc << 1) + (crc >> 7) ^ (byt >> i & 1)
		}
	}
	return crc
}
