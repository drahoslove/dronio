// Module fly implements functionality of remote controller for visuo drone family (xs809s, xs809, xs809w, xs809h, xs809hw,...)
//
// Usage
//
//  - use Start() and Halt() to turn on/off the transmitter
//  - use Calibrate() to calibrate the gyro before flight
//  - use CompassOn() and CompassOff() to turn on/off the headless mode
//  - use TakeOff() and Land() to get the drone to air and back on the ground
//  - use Sticks(up, rotate, forwards, sideways) and Hover() to control the flight
//  - use Flip() to prepare for flip
//  - use Stop() to emergency stop
//
//
//  Following commands blocks for .5s:
//  - use GoUp(speed), GoDown(speed), GoLeft(speed), GoRight(speed), GoClockwise(speed), GoCounterClockwise(speed) to move in direction in steps
//  - use DoBackFlip(), DoFrontFlip(), DoRightFlip() and DoLeftFlip() to do various flips
//
//
// Caution:
//
// do not get confuse following methods!
//
// Halt() = turn off radio transmitting
//  - opposite of `Start()`
//  - makes drone unresponsive to any subsequent commands
//  - should be only called at the end of the session, when drone is safely on ground whith propellers not spinning
//
// Hover() = reset sticks to neutral position
//  - stops drone from accelerating when flying
//  - but it will not hapen instantly (because inertia)
//  - it will also not freeze drone in place entirely (because wind, turbulences, and gyro imperfections)
//
// Stop() = stop propellers
//  - drone itself will accelerate towards ground due to gravity
//  - should be used in case of emergency when crash is unavoidable to prevent damage or injuries from rotating propellers
//
// Land() = land drone kind of safely
//  - opposite to `TakeOff()`
//  - slowly decreases speed of the propellers and then stops it entirely
//  - it can be harsh, so drone should be already close to ground (<1m) when used
//
//
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

func (c *Cmd) String() (str string) {
	for _, b := range c.data {
		str += fmt.Sprintf("%02x ", b)
	}
	return
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
// it is here to satisfy gobot.Driver intreface
func (d *Driver) Name() string {
	return d.name
}

// Name will set the name of the driver instance
//
// It is here to satisfy gobot.Driver interface
func (d *Driver) SetName(name string) {
	d.name = name
}

// Connection is not actually useful so far
//
// it is only here to satisfy gobot.Driver itnerface
func (d *Driver) Connection() gobot.Connection {
	return nil
}

// Start will start transmitting loop
//
// Similar to turning on the remote controll
func (d *Driver) Start() error {
	d.Lock()
	defer d.Unlock()
	d.reset()
	if !d.enabled {
		d.radioLoop()
	}
	return d.err
}

// Halt will end transmitting loop
//
// Similar to turning off the remote controll
func (d *Driver) Halt() error {
	d.Lock()
	defer d.Unlock()
	if d.enabled {
		d.stop <- true
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
		return
	}
	d.enabled = true

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
				d.enabled = false
				return
			default:
			}
		}
	}()

}

// Reset cmd to default state
func (d *Driver) reset() {
	d.cmd.update(func(data []byte) {
		data[1] = normalize(0)
		data[2] = normalize(0)
		data[3] = normalize(0)
		data[4] = normalize(0)
		data[5] = 0
	})
}

/* Stick controll commands */

// Sticks commands drone to fly according to sticks position
//
//                      -1.0 … +1.0
//  up       (throttle)    ↓ … ↑
//  rotate   (yaw)         ↶ … ↷
//  forwards (pitch)       ▼ … ▲
//  sideways (roll)        ◀ … ▶
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

// Hover commands drone to stop movement and hover in place.
// (It sets sticks to rest positions.)
//
// Same as d.Sticks(0,0,0,0)
func (d *Driver) Hover() {
	d.cmd.update(func(data []byte) {
		data[rollByte] = normalize(0)
		data[pitchByte] = normalize(0)
		data[throttleByte] = normalize(0)
		data[yawByte] = normalize(0)
	})
}

// Up makes the drone gain altitude.
// speed foat can be a value from `0` to `1`.
func (d *Driver) GoUp(speed float64) {
	d.cmd.update(func(d []byte) { d[throttleByte] = normalize(speed / +1) })
	time.Sleep(time.Second / 2)
	d.Hover()
}

// Down makes the drone reduce altitude.
// speed can be a foat value from `0` to `1`.
func (d *Driver) GoDown(speed float64) {
	d.cmd.update(func(d []byte) { d[throttleByte] = normalize(speed / -1) })
	time.Sleep(time.Second / 2)
	d.Hover()
}

// Right causes the drone to bank to the right, controls the roll.
// speed can be a foat value from `0` to `1`.
func (d *Driver) GoRight(speed float64) {
	d.cmd.update(func(d []byte) { d[rollByte] = normalize(speed / +1) })
	time.Sleep(time.Second / 2)
	d.Hover()
}

// Left causes the drone to bank to the left, controls the roll.
// speed can be a foat value from `0` to `1`.
func (d *Driver) GoLeft(speed float64) {
	d.cmd.update(func(d []byte) { d[rollByte] = normalize(speed / -1) })
	time.Sleep(time.Second / 2)
	d.Hover()
}

// Forward causes the drone go forward, controls the pitch.
// speed can be a foat value from `0` to `1`.
func (d *Driver) GoForward(speed float64) {
	d.cmd.update(func(d []byte) { d[pitchByte] = normalize(speed / +1) })
	time.Sleep(time.Second / 2)
	d.Hover()
}

// Backward causes the drone go forward, controls the pitch.
// speed can be a foat value from `0` to `1`.
func (d *Driver) GoBackward(speed float64) {
	d.cmd.update(func(d []byte) { d[pitchByte] = normalize(speed / -1) })
	time.Sleep(time.Second / 2)
	d.Hover()
}

// Clockwise tells drone to rotate in a clockwise direction.
// speed can be a float value from `0` to `1`.
func (d *Driver) GoClockwise(speed float64) {
	d.cmd.update(func(d []byte) { d[yawByte] = normalize(speed / -1) })
	time.Sleep(time.Second / 2)
	d.Hover()
}

// Clockwise tells drone to rotate in a clockwise direction.
// speed can be a float value from `0` to `1`.
func (d *Driver) GoCounterClockwise(speed float64) {
	d.cmd.update(func(d []byte) { d[yawByte] = normalize(speed / +1) })
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
func (d *Driver) DoBackFlip() {
	d.Flip()
	d.GoBackward(100)
}

// FrontFlip commands drone to do a frontflip
func (d *Driver) DoFrontFlip() {
	d.Flip()
	d.GoForward(100)
}

// LeftFlip commands drone to do a flip to the left
func (d *Driver) DoLeftFlip() {
	d.Flip()
	d.GoLeft(100)
}

// RightFlip commands drone to do a flip to the right
func (d *Driver) DoRightFlip() {
	d.Flip()
	d.GoRight(100)
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
