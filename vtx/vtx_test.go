package vtx

import (
	"testing"
	"time"
)

func TestTakePhoto(t *testing.T) {
	// TakePhoto()
}

func TestCaptureVideo(t *testing.T) {
	SetClock()
	// TakePhoto()
	// ListVideos()

	println("video capture started")
	CaptureVideo(20 * time.Second)
	println("video capture ended")
	time.Sleep(time.Second * 2)
	videos := ListVideos()
	println("videos listed")
	for _, video := range videos {
		println("downloading video", video.Filename)
		t1 := time.Now()
		DownloadVideo(video.Filename)
		println("saving videoreplay")
		ReplayVideo(video.Filename)
		println(time.Now().Sub(t1).String())
		time.Sleep(time.Second * 2)
		println("deleting video", video.Filename)
		DeleteVideo(video.Filename)
		println("done")
	}
}
