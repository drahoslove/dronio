package main

import (
	"log"
	"math/rand"
	"time"

	"golang.org/x/mobile/app"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/event/size"
	"golang.org/x/mobile/event/touch"
	"golang.org/x/mobile/exp/app/debug"
	"golang.org/x/mobile/exp/gl/glutil"
	"golang.org/x/mobile/gl"

	"github.com/drahoslove/dronio/fly"
	_ "github.com/drahoslove/dronio/vtx"
)

var (
	images *glutil.Images
	fps    *debug.FPS
)

func main() {
	app.Main(func(a app.App) {
		var glctx gl.Context
		var sz size.Event
		var err error

		prolongErr := reAfterFunc(time.Second/4, func() {
			err = nil
		})
		fly := fly.NewDriver("192.168.0.1:50000")
		fly.OnError(func(e error) {
			err = e
			prolongErr()
		})

		for e := range a.Events() {
			switch e := a.Filter(e).(type) {
			case lifecycle.Event:
				switch e.Crosses(lifecycle.StageVisible) {
				case lifecycle.CrossOn:
					fly.Start()
					// d.Default()
					// time.AfterFunc(time.Second*2, func() {
					// 	d.Controls(-1, 0, 0, 0)
					// })
					time.AfterFunc(time.Second*4, func() {
						fly.Calibrate()
					})
					// a.Send(paint.Event{})
				case lifecycle.CrossOff:
					fly.Halt()
				}
				switch e.Crosses(lifecycle.StageAlive) {
				case lifecycle.CrossOn:
					glctx, _ = e.DrawContext.(gl.Context)
					onStart(glctx)
					a.Send(paint.Event{})
				case lifecycle.CrossOff:
					onStop(glctx)
				}
			case size.Event:
				println("size event")
				sz = e
				// a.Send(paint.Event{})
			case touch.Event:
				if e.Type == touch.TypeBegin {
					log.Println("Touch at", e.X, e.Y)
				}
				// a.Send(paint.Event{})
			case paint.Event:
				if e.External || glctx == nil {
					continue
				}
				onDraw(glctx, sz, err)
				a.Publish()
				a.Send(paint.Event{})
			}
		}
	})
}

func onStart(glctx gl.Context) {
	images = glutil.NewImages(glctx)
	fps = debug.NewFPS(images)
}

func onStop(glctx gl.Context) {
	fps.Release()
	images.Release()
}

func onDraw(glctx gl.Context, sz size.Event, err error) {
	if err == nil {
		r := 1 - rand.Float32()/8
		g := 1 - rand.Float32()/2
		b := 1 - rand.Float32()/8
		glctx.ClearColor(r, g, b, 1)
	} else {
		glctx.ClearColor(1, 0, 0, 1)
	}
	glctx.Clear(gl.COLOR_BUFFER_BIT)
	fps.Draw(sz)
}

// Runs fn after given time from calling returned reset func
// reset sets new timer and cancles previous if any is ticking
func reAfterFunc(duration time.Duration, fn func()) (reset func()) {
	var ticker *time.Timer
	reset = func() {
		if ticker != nil && !ticker.Stop() {
			<-ticker.C
		}
		ticker = time.AfterFunc(duration, fn)
	}
	return
}
