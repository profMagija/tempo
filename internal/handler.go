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

var sampleRate = 10 * time.Millisecond

func WriteTrace(ctx context.Context, duration time.Duration, w io.Writer) error {
	prof := &profile.Profile{
		DurationNanos: int64(duration),
		SampleType: []*profile.ValueType{
			{Type: "samples", Unit: "count"},
			{Type: "wall", Unit: "nanoseconds"},
		},
		Period: int64(sampleRate),
		PeriodType: &profile.ValueType{
			Type: "wall",
			Unit: "nanoseconds",
		},
	}

	pb := &profileBuilder{
		prof:          prof,
		sampleCache:   make(map[[32]uintptr]*profile.Sample),
		locationCache: make(map[uintptr]*profile.Location),
		functionCache: make(map[string]*profile.Function),
	}

	totalCount := int(duration / sampleRate)

	for i := 0; i < totalCount; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		sleepTo := time.Now().Add(sampleRate)

		stkProf := getProfile()
		pb.addProfile(stkProf)

		time.Sleep(time.Until(sleepTo))
	}

	_ = prof.Write(w)
	return nil
}

type profileBuilder struct {
	prof *profile.Profile

	sampleCache   map[[32]uintptr]*profile.Sample
	locationCache map[uintptr]*profile.Location
	functionCache map[string]*profile.Function
}

func (pb *profileBuilder) addProfile(sProfile stackProfile) {
	for i := 0; i < sProfile.Len(); i++ {
		sample, ok := pb.sampleCache[sProfile.Stack0(i)]
		if ok {
			sample.Value[0]++
			sample.Value[1] += int64(sampleRate)
			continue
		} else {
			pb.createSample(sProfile, i)
		}
	}
}

func (pb *profileBuilder) createSample(sProfile stackProfile, i int) {
	sample := &profile.Sample{}

	sample.Value = []int64{1, int64(sampleRate)}

	// label := stackProfile.Label(i)
	// if label != nil {
	// 	sample.Label = make(map[string][]string)
	// 	for k, v := range *label {
	// 		sample.Label[k] = []string{v}
	// 	}
	// }

	frames := runtime.CallersFrames(sProfile.Stack(i))

	for {
		frame, more := frames.Next()
		if !more {
			// we intentionally ignore the last frame
			break
		}

		loc := pb.getOrCreateLocation(&frame)
		sample.Location = append(sample.Location, loc)
	}

	pb.sampleCache[sProfile.Stack0(i)] = sample
	pb.prof.Sample = append(pb.prof.Sample, sample)
}

func (pb *profileBuilder) getOrCreateLocation(frame *runtime.Frame) *profile.Location {
	loc, ok := pb.locationCache[frame.PC]
	if ok {
		return loc
	}

	function := pb.getOrCreateFunction(frame.Func, frame.Function, frame.File)

	loc = &profile.Location{
		ID:      uint64(len(pb.prof.Location) + 1),
		Address: uint64(frame.PC),
		Line:    []profile.Line{{Function: function, Line: int64(frame.Line)}},
	}

	pb.locationCache[frame.PC] = loc
	pb.prof.Location = append(pb.prof.Location, loc)

	return loc
}

func (pb *profileBuilder) getOrCreateFunction(f *runtime.Func, name string, filename string) *profile.Function {
	function, ok := pb.functionCache[name]
	if ok {
		return function
	}

	var startline int
	if f != nil {
		filename, startline = f.FileLine(f.Entry())
	}

	function = &profile.Function{
		ID:         uint64(len(pb.prof.Function) + 1),
		Name:       name,
		SystemName: name,
		Filename:   filename,
		StartLine:  int64(startline),
	}

	pb.functionCache[name] = function
	pb.prof.Function = append(pb.prof.Function, function)

	return function
}
