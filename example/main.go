package main

import (
	"net/http"
	"runtime"
	"sync"
	"time"

	_ "net/http/pprof"

	_ "github.com/profmagija/tempo/http"
)

func sleepLoop() {
	for {
		time.Sleep(1 * time.Second)
	}
}

func spinLoop() {
	for {
	}
}

func schedLoop() {
	for {
		runtime.Gosched()
	}
}

func mutexLock(mutex *sync.Mutex) {
	mutex.Lock()
}

func chanWrite(ch chan int) {
	ch <- 1
}

func chanRead(ch chan int) {
	<-ch
}

func main() {
	go sleepLoop()

	go spinLoop()

	go schedLoop()

	mutex := new(sync.Mutex)
	mutex.Lock()
	go mutexLock(mutex)

	go chanWrite(make(chan int))

	go chanRead(make(chan int))

	go func() {
		panic(http.ListenAndServe(":6060", nil))
	}()

	select {}
}
