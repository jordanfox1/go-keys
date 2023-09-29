package keys

import (
	"flag"
	"fmt"
	"io"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

var (
	sampleRate   = flag.Int("samplerate", 44100, "sample rate")
	channelCount = flag.Int("channelcount", 10, "number of channel")
	format       = flag.String("format", "s16le", "source format (u8, s16le, or f32le)")
	NoteCount    = 0 // Exported global variable
)

type SineWave struct {
	freq   float64
	length int64
	pos    int64

	channelCount int
	format       oto.Format

	remaining []byte
}

func formatByteLength(format oto.Format) int {
	switch format {
	case oto.FormatFloat32LE:
		return 4
	case oto.FormatUnsignedInt8:
		return 1
	case oto.FormatSignedInt16LE:
		return 2
	default:
		panic(fmt.Sprintf("unexpected format: %d", format))
	}
}

func NewSineWave(freq float64, duration time.Duration, channelCount int, format oto.Format) *SineWave {
	l := int64(channelCount) * int64(formatByteLength(format)) * int64(*sampleRate) * int64(duration) / int64(time.Second)
	l = l / 4 * 4
	return &SineWave{
		freq:         freq,
		length:       l,
		channelCount: channelCount,
		format:       format,
	}
}

func (s *SineWave) Read(buf []byte) (int, error) {
	if len(s.remaining) > 0 {
		n := copy(buf, s.remaining)
		copy(s.remaining, s.remaining[n:])
		s.remaining = s.remaining[:len(s.remaining)-n]
		return n, nil
	}

	if s.pos == s.length {
		return 0, io.EOF
	}

	eof := false
	if s.pos+int64(len(buf)) > s.length {
		buf = buf[:s.length-s.pos]
		eof = true
	}

	var origBuf []byte
	if len(buf)%4 > 0 {
		origBuf = buf
		buf = make([]byte, len(origBuf)+4-len(origBuf)%4)
	}

	length := float64(*sampleRate) / float64(s.freq)

	num := formatByteLength(s.format) * s.channelCount
	p := s.pos / int64(num)
	switch s.format {
	case oto.FormatFloat32LE:
		for i := 0; i < len(buf)/num; i++ {
			bs := math.Float32bits(float32(math.Sin(2*math.Pi*float64(p)/length) * 0.3))
			for ch := 0; ch < *channelCount; ch++ {
				buf[num*i+4*ch] = byte(bs)
				buf[num*i+1+4*ch] = byte(bs >> 8)
				buf[num*i+2+4*ch] = byte(bs >> 16)
				buf[num*i+3+4*ch] = byte(bs >> 24)
			}
			p++
		}
	case oto.FormatUnsignedInt8:
		for i := 0; i < len(buf)/num; i++ {
			const max = 127
			b := int(math.Sin(2*math.Pi*float64(p)/length) * 0.3 * max)
			for ch := 0; ch < *channelCount; ch++ {
				buf[num*i+ch] = byte(b + 128)
			}
			p++
		}
	case oto.FormatSignedInt16LE:
		for i := 0; i < len(buf)/num; i++ {
			const max = 32767
			b := int16(math.Sin(2*math.Pi*float64(p)/length) * 0.3 * max)
			for ch := 0; ch < *channelCount; ch++ {
				buf[num*i+2*ch] = byte(b)
				buf[num*i+1+2*ch] = byte(b >> 8)
			}
			p++
		}
	}

	s.pos += int64(len(buf))

	n := len(buf)
	if origBuf != nil {
		n = copy(origBuf, buf)
		s.remaining = buf[n:]
	}

	if eof {
		return n, io.EOF
	}
	return n, nil
}

func play(context *oto.Context, freq float64, duration time.Duration, channelCount int, format oto.Format) *oto.Player {
	p := context.NewPlayer(NewSineWave(freq, duration, channelCount, format))
	p.Play()
	return p
}

func InitAudioContext() (*oto.Context, *oto.NewContextOptions, error) {
	op := &oto.NewContextOptions{}
	op.SampleRate = *sampleRate
	op.ChannelCount = *channelCount

	switch *format {
	case "f32le":
		op.Format = oto.FormatFloat32LE
	case "u8":
		op.Format = oto.FormatUnsignedInt8
	case "s16le":
		op.Format = oto.FormatSignedInt16LE
	default:
		return nil, nil, fmt.Errorf("format must be u8, s16le, or f32le but: %s", *format)
	}

	c, ready, err := oto.NewContext(op)
	if err != nil {
		return nil, nil, err
	}
	<-ready
	return c, op, nil
}

var noteFrequencies = map[string]float64{
	"q": 523.25,  // C5
	"2": 554.37,  // C#5
	"w": 587.33,  // D5
	"3": 622.25,  // D#5
	"e": 659.25,  // E5
	"4": 698.46,  // F5
	"r": 739.99,  // F#5
	"5": 783.99,  // G5
	"t": 830.61,  // G#5
	"6": 880.00,  // A5
	"y": 932.33,  // A#5
	"7": 987.77,  // B5
	"u": 1046.50, // C6
	"8": 1108.73, // C#6
	"i": 1174.66, // D6
	"9": 1244.51, // D#6
	"o": 1318.51,
	"z": 261.63, // C4
	"s": 277.18, // C#4
	"x": 293.66, // D4
	"d": 311.13, // D#4
	"c": 329.63, // E4
	"f": 349.23, // F4
	"v": 369.99, // F#4
	"g": 392.00, // G4
	"b": 415.30, // G#4
	"h": 440.00, // A4
	"n": 466.16, // A#4
	"j": 493.88, // B4
}

func Run(key rune, c *oto.Context, op *oto.NewContextOptions) error {
	var wg sync.WaitGroup
	var players []*oto.Player
	var m sync.Mutex

	// Map keys to corresponding frequencies
	keyStr := string(key)
	if freq, ok := noteFrequencies[keyStr]; ok {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := play(c, freq, 3*time.Second, op.ChannelCount, op.Format)
			var initialVolume float64 = 1 / float64(NoteCount)

			p.SetVolume(initialVolume)
			time.Sleep(1 * time.Second)
			p.SetVolume(initialVolume / 2)
			time.Sleep(1 * time.Second)
			p.SetVolume(initialVolume / 3)
			time.Sleep(50000)
			p.SetVolume(0.0)

			m.Lock()
			players = append(players, p)
			m.Unlock()
			NoteCount--
		}()

		wg.Wait()
		// Pin the players not to GC the players.
		runtime.KeepAlive(players)
	}

	return nil
}
