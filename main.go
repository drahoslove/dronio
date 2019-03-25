package main

import (
	"encoding/binary"
	"log"
	"time"

	"golang.org/x/mobile/app"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/event/size"
	"golang.org/x/mobile/event/touch"
	"golang.org/x/mobile/exp/app/debug"
	"golang.org/x/mobile/exp/f32"
	"golang.org/x/mobile/exp/gl/glutil"
	"golang.org/x/mobile/gl"

	"github.com/drahoslove/dronio/fly"
	_ "github.com/drahoslove/dronio/vtx"
)

var (
	images   *glutil.Images
	fps      *debug.FPS
	program  gl.Program
	offset   gl.Uniform
	position gl.Attrib
	color    gl.Uniform
	buf      gl.Buffer
	bufi     gl.Buffer
	touchX   float32
	touchY   float32
)

var vertices = f32.Bytes(binary.LittleEndian,
	-1, -1, 0, // bottom left corner
	-1, 1, 0, // top left corner
	1, 1, 0, // top right corner
	1, -1, 0, // bottom right corner
)

var indices = []byte{
	0, 1, 2, // first triangle (bottom left - top left - top right)
	0, 2, 3, // second triangle (bottom left - top right - bottom right)
}

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
				touchX = e.X
				touchY = e.Y
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
	glctx.Disable(gl.DEPTH_TEST)
	glctx.Enable(gl.BLEND)
	glctx.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	// create program
	var err error
	program, err = glutil.CreateProgram(glctx, vertexShader, fragmentShader)
	if err != nil {
		log.Printf("error creating gl program: %v", err)
		return
	}

	// create buffer
	buf = glctx.CreateBuffer()
	glctx.BindBuffer(gl.ARRAY_BUFFER, buf)
	glctx.BufferData(gl.ARRAY_BUFFER, vertices, gl.STATIC_DRAW)
	bufi = glctx.CreateBuffer()
	glctx.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, bufi)
	glctx.BufferData(gl.ELEMENT_ARRAY_BUFFER, indices, gl.STATIC_DRAW)

	// set gl variables
	position = glctx.GetAttribLocation(program, "position")
	color = glctx.GetUniformLocation(program, "color")
	offset = glctx.GetUniformLocation(program, "offset")

	images = glutil.NewImages(glctx)
	fps = debug.NewFPS(images)
}

func onStop(glctx gl.Context) {
	glctx.DeleteProgram(program)
	glctx.DeleteBuffer(buf)
	fps.Release()
	images.Release()
}

func onDraw(glctx gl.Context, sz size.Event, err error) {
	glctx.ClearColor(1, 0, 0, 1) // red backgroundin
	glctx.Clear(gl.COLOR_BUFFER_BIT)
	glctx.UseProgram(program)

	glctx.Uniform4f(color, 0.9, 0.9, 0.9, 1.0) // whiteish grey/
	glctx.Uniform2f(offset, touchX/float32(sz.WidthPx), touchY/float32(sz.HeightPx))

	glctx.BindBuffer(gl.ARRAY_BUFFER, buf)
	glctx.EnableVertexAttribArray(position)
	glctx.VertexAttribPointer(position, 3, gl.FLOAT, true, 0, 0) // 4vec attr, 3 coords per

	glctx.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, bufi)
	glctx.DrawElements(gl.TRIANGLES, len(indices), gl.UNSIGNED_BYTE, 0) // 6 vertices

	glctx.DisableVertexAttribArray(position)
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

const vertexShader = `#version 100
uniform vec2 offset; // 0.0-1.0
attribute vec4 position;

varying vec2 vertPos;

void main(){
	vec4 offset4 = vec4(2.0*offset.x-1.0, -(2.0*offset.y-1.0), 0, 0);
	gl_Position = position + offset4;
	vertPos = position.xy;
}
`

const fragmentShader = `#version 100
precision mediump float; // ???

uniform vec4 color;

varying vec2 vertPos;

void main(){
	gl_FragColor = color; // some color
	if ((vertPos.x * vertPos.x) + (vertPos.y * vertPos.y) >= 1.0) {
		gl_FragColor.a = 0.1; // transparent
	}
}
`
