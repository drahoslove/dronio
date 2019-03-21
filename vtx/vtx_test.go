package vtx

import (
	"testing"
)

func TestTakePhoto(t *testing.T) {
	// TakePhoto()
}

func TestCaptureVideo(t *testing.T) {
	ListVideos()
	DeleteVideo("a:/Video/20181206_230946.mp4")
	DownloadVideo("a:/Video/20181205_225611.mp4")
	ListVideos()

	// CaptureVideo(10 * time.Second)
}
