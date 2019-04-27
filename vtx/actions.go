package vtx

import (
	"bytes"
	"encoding/binary"
	"fmt"
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
	filename string
	duration uint32
}) {
	Action(listVideosCmd, nil, func(payload []byte) {
		for ; len(payload) > 0; payload = payload[116:] {
			duration := binary.LittleEndian.Uint32(payload[4:8])
			filename := string(bytes.Trim(payload[4*4:4*4+100], "\x00"))
			videos = append(videos, struct {
				filename string
				duration uint32
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
		data := Res(videoFileCmd, conn)
		data32 := byteToUint32(data)
		chunkSize := int(data32[1])
		fileSize := int(data32[2])

		// check if this is data for requested file
		if !bytes.Equal(payload[4*4:4*4+100], data[4*4:4*4+100]) {
			fmt.Printf("%v\n%v\n", fmt.Errorf("Can't download this video - bad response"), data[:len(payload)])
			return
		}

		switch data32[0] { // first number is type of data (1 = start, 2 = data, 3 = end)
		case 1: // start
			// create empty file
			file, err := os.OpenFile(filepath.Base(fileName), os.O_CREATE|os.O_WRONLY, 0777)
			if err != nil {
				fmt.Printf("%v %v\n%v\n", fmt.Errorf("Can't crate video file"), fileName, err)
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
			fmt.Printf("%d%%\n", bytesLoaded*100/fileSize)
			println("checksum:", chunkSize, bytesLoaded, fileSize, string(data[116:]))
			if bytesLoaded == fileSize {
				break loop
			}
			println("Not whole file recieved")
			// TODO check checksum
		default:
			fmt.Printf("wrong state %v\n", data32)
			break loop
		}
	}
	println("done")
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
