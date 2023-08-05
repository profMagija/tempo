package internal

import (
	"context"
	"io"
	"runtime"
	"time"
	"unsafe"

	"github.com/google/pprof/profile"
)

type labelMap map[string]string

type stackProfile struct {
	stack  []runtime.StackRecord
	labels []unsafe.Pointer
}

func (p *stackProfile) Len() int {
	return len(p.stack)
}

func (p *stackProfile) Stack0(i int) [32]uintptr {
	return p.stack[i].Stack0
}

func (p *stackProfile) Stack(i int) []uintptr {
	return p.stack[i].Stack()
}

func (p *stackProfile) Label(i int) *labelMap {
	return (*labelMap)(p.labels[i])
}

//go:linkname goroutineProfileWithLabels runtime/pprof.runtime_goroutineProfileWithLabels
func goroutineProfileWithLabels(p []runtime.StackRecord, labels []unsafe.Pointer) (n int, ok bool)

func getProfile() stackProfile {
	// Find out how many records there are (fetch(nil)),
	// allocate that many records, and get the data.
	// There's a race—more records might be added between
	// the two calls—so allocate a few extra records for safety
	// and also try again if we're very unlucky.
	// The loop should only execute one iteration in the common case.
	var p []runtime.StackRecord
	var labels []unsafe.Pointer
	n, _ := goroutineProfileWithLabels(nil, nil)
	for {
		// Allocate room for a slightly bigger profile,
		// in case a few more entries have been added
		// since the call to ThreadProfile.
		p = make([]runtime.StackRecord, n+10)
		labels = make([]unsafe.Pointer, n+10)

		var ok bool
		n, ok = goroutineProfileWithLabels(p, labels)
		if ok {
			p = p[0:n]
			break
		}
		// Profile grew; try again.
	}

	return stackProfile{p, labels}
}

func WriteTrace(ctx context.Context, duration time.Duration, w io.Writer) error {
	var profiles []stackProfile

	sampleRate := 10 * time.Millisecond

	end := time.Now().Add(duration)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		sleepTo := time.Now().Add(sampleRate)
		profiles = append(profiles, getProfile())

		if time.Now().After(end) {
			break
		}

		time.Sleep(time.Until(sleepTo))
	}

	locCache := make(map[uintptr]*profile.Location)
	funcCache := make(map[string]*profile.Function)
	sampCache := make(map[[32]uintptr]*profile.Sample)

	var prof profile.Profile

	prof.DurationNanos = int64(duration)

	prof.SampleType = []*profile.ValueType{
		{Type: "samples", Unit: "count"},
		{Type: "wall", Unit: "nanoseconds"},
	}

	prof.Period = int64(sampleRate)
	prof.PeriodType = &profile.ValueType{Type: "wall", Unit: "nanoseconds"}

	for _, stackProfile := range profiles {
		for i := 0; i < stackProfile.Len(); i++ {
			samp, ok := sampCache[stackProfile.Stack0(i)]
			if ok {
				samp.Value[0]++
				samp.Value[1] += int64(sampleRate)
				continue
			}

			samp = &profile.Sample{}

			samp.Value = []int64{1, int64(sampleRate)}

			// label := stackProfile.Label(i)
			// if label != nil {
			// 	samp.Label = make(map[string][]string)
			// 	for k, v := range *label {
			// 		samp.Label[k] = []string{v}
			// 	}
			// }

			frames := runtime.CallersFrames(stackProfile.Stack(i))

			for {
				frame, more := frames.Next()
				if !more {
					break
				}

				loc, ok := locCache[frame.PC]
				if !ok {
					funct, ok := funcCache[frame.Function]
					if !ok {
						funct = &profile.Function{
							ID:   uint64(len(prof.Function) + 1),
							Name: frame.Function,
						}

						funcCache[frame.Function] = funct
						prof.Function = append(prof.Function, funct)
					}

					loc = &profile.Location{
						ID:      uint64(len(prof.Location) + 1),
						Address: uint64(frame.PC),
						Line:    []profile.Line{{Function: funct, Line: int64(frame.Line)}},
					}

					locCache[frame.PC] = loc
					prof.Location = append(prof.Location, loc)
				}

				samp.Location = append(samp.Location, loc)
			}

			sampCache[stackProfile.Stack0(i)] = samp
			prof.Sample = append(prof.Sample, samp)
		}
	}

	_ = prof.Write(w)
	return nil
}
