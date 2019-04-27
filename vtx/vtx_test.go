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
	// ListVideos()
	// DeleteVideo("a:/Video/20181206_230946.mp4")
	// DownloadVideo("a:/Video/20181205_225611.mp4")
	// ListVideos()

	// TakePhoto()
	// ListVideos()

	println("video capture started")
	// CaptureVideo(400 * time.Second)
	println("video capture ended")
	time.Sleep(time.Second * 2)
	videos := ListVideos()
	for _, video := range videos {
		println("downloading video", video.filename)
		t1 := time.Now()
		// DownloadVideo(video.filename)
		ReplayVideo(video.filename)
		println(time.Now().Sub(t1).String())
		time.Sleep(time.Second * 2)
		println("deleting video", video.filename)
		// DeleteVideo(video.filename)
	}
}
