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

	CaptureVideo(10 * time.Second)
	// time.Sleep(time.Second)
	// ListVideos()

}
