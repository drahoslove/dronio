package vtx

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

var (
	localOffset = int64(0) // in seconds
	chinaOffset = int64(8 * 60 * 60)
)

func init() {
	_, offset := time.Now().Local().Zone()
	localOffset = int64(offset)
}

// SetClock sets internal clock of the drone to currnet time (for saving files by actuall current date)
func SetClock() {
	timestamp := uint32(time.Now().Unix() + localOffset - chinaOffset)
	data := []uint32{timestamp, 0}
	Action(setClockCmd, data, nil)
}

// TakePhoto will take photo and save to current dir
func TakePhoto() {
	Action(takePhotoCmd, nil, func(payload []byte) {
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

func ListVideos() (videos []struct {
	Filename string
	Duration uint32
}) {
	Action(listVideosCmd, nil, func(payload []byte) {
		for ; len(payload) > 0; payload = payload[116:] {
			duration := binary.LittleEndian.Uint32(payload[4:8])
			filename := string(bytes.Trim(payload[4*4:4*4+100], "\x00"))
			videos = append(videos, struct {
				Filename string
				Duration uint32
			}{filename, duration})
		}
	})
	return
}

// DeleteVideo deletes video by given name
func DeleteVideo(filename string) {
	payload := make([]byte, 100)
	copy(payload, filename)
	Action(deleteVideoCmd, payload, nil)
}

// DownloadVideo will dowlnoad video by given name
func DownloadVideo(fileName string) {
	// create custom connection because we cant use Action in this case
	conn, closeConn := newConn(portByCmd(downloadVideoCmd))
	if conn == nil {
		return
	}
	defer closeConn()

	// send Req for downloading video
	payload := make([]byte, 196)
	copy(payload[4*4:], fileName)
	Req(downloadVideoCmd, payload, conn)

	file := &os.File{}
	bytesLoaded := 0
loop:
	for { // obtain responses
		data := Res(videoDownloadCmd, conn)
		data32 := byteToUint32(data)
		chunkSize := int(data32[1])
		fileSize := int(data32[2])
		recvFileName := string(bytes.Trim(data[4*4:4*4+100], "\x00"))

		// check if this is data for requested file
		if recvFileName != fileName {
			panic(fmt.Errorf("%v\n%v\n", fmt.Errorf("Can't download this video - bad response"), data[:len(payload)]))
			return
		}

		switch data32[0] { // first number is type of data (1 = start, 2 = data, 3 = end)
		case 1: // start
			// create empty file
			err := error(nil)
			file, err = os.OpenFile(filepath.Base(fileName), os.O_CREATE|os.O_WRONLY, 0777)
			if err != nil {
				panic(fmt.Errorf("%v %v\n%v\n", fmt.Errorf("Can't crate video file"), fileName, err))
				return
			}
			defer file.Close()
		case 2: // load data chunks
			// the rest is the file itself
			chunkContent := data[len(payload) : len(payload)+chunkSize]
			// save file content to current directory
			_, err := file.Write(chunkContent)
			if err != nil {
				panic(err)
			}
			bytesLoaded += chunkSize
		case 3: // end
			// fmt.Printf("%d%%\n", bytesLoaded*100/fileSize)
			println("checksum:", chunkSize, bytesLoaded, fileSize, string(data[116:]))
			if bytesLoaded == fileSize {
				break loop
			}
			println("Not whole file recieved")
			// TODO check checksum
		default:
			println("!!!wrong state", data32)
			break loop
		}
	}
	// println("done")
}

func ReplayVideo(fileName string, output io.Writer) {
	// create custom connection because we cant use Action in this case
	conn, closeConn := newConn(portByCmd(downloadVideoCmd))
	if conn == nil {
		return
	}
	defer closeConn()

	payload := make([]byte, 124)
	payload32 := byteToUint32(payload)
	payload32[1] = 0x0000003a // ??
	copy(payload[2*4:4*18], "_lewei_lib_Lewei"+fileName+"\x00ava_lang_String_2III")
	// no idea what these are for:
	// payload32[19] = 0x00006300
	// payload32[21] = 0x00001a00
	// payload32[25] = 0xff002000
	// payload32[27] = 0xffffff00
	// payload32[29] = 0xffffff00
	// fmt.Printf("% x\n", payload)

	// file, _ := os.OpenFile("replay"+filepath.Base(fileName)+".h264", os.O_CREATE|os.O_WRONLY, 0777)
	// defer file.Close()

	Req(replayVideoCmd, payload, conn)
	const fps = 20

	ticker := time.NewTicker(time.Second / fps)
	defer ticker.Stop()

	for {
		<-ticker.C

		// incoming()
		data := Res(videoReplayCmd, conn)
		data32 := byteToUint32(data)
		if len(data) == 0 {
			println("eend")
			// Req(closeCmd, nil, conn)
			return
		}
		// 4 x uint32 chunk header:
		chunkType := data32[0] // 1 or 0 sometimes 256
		// 1 is key frame (~40-90kB) every 40th (every 2s)
		// 0 is delta frame (~1-20kB)
		chunkSize := data32[1]
		_ = data[2] // seems to be always zero
		// chunkTime := data32[3] // multiples of 50
		chunkContent := data[32:]

		if chunkSize == 0 {
			println("end", data32[0])
			// Req(closeCmd, nil, conn)
			return
		}

		if chunkType != 1 && chunkType != 0 {
			println("!!!chunktype", chunkType)
			return
		}

		// another layer with 4 x 16uint values
		frame := binary.LittleEndian.Uint16(chunkContent[0:2])  // seq number of frame
		ff := binary.LittleEndian.Uint16(chunkContent[2:4])     // seq number of frame
		timing := binary.LittleEndian.Uint16(chunkContent[4:6]) // same as chunkTime
		println(frame, ff, timing)
		if ff == 0xff00 {
			continue
		}

		if output != nil {
			output.Write(chunkContent[8:])
		}
	}
}

// CaptureVideo will capture video of given period of time
func CaptureVideo(duration time.Duration) {
	StartVideo()
	time.Sleep(duration)
	StopVideo()
}

// StartVideo will start video recording (unless it already started)
func StartVideo() {
	if !IsCapturing() {
		// Action(captureVideoCmd, []uint32{on, 4, 0, 24*60*60 - 1, 5 * 60}, nil)
		Action(captureVideoCmd, []uint32{on, 0, 0, 0, 0}, nil)
	}
}

// StopVideo will stop video recording (unless it already stopped)
func StopVideo() {
	if IsCapturing() {
		// Action(captureVideoCmd, []uint32{off, 4, 0, 24*60*60 - 1, 5 * 60}, nil)
		Action(captureVideoCmd, []uint32{off, 0, 0, 0, 0}, nil)
	}
}

// IsCapturing will fetch payload last set by StartVide/StopVideo and reurn boolean accordingly
func IsCapturing() bool {
	isCapturing := false
	Action(checkVideoCmd, nil, func(payload []byte) {
		capturing := byteToUint32(payload)[0]
		isCapturing = capturing == on
	})
	return isCapturing
}
