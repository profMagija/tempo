# `tempo` - a missing piece of `pprof` timing arsenal

`tempo` is a library to expose the total *wall* timing of your program. This will include the time spent in system calls, waiting for I/O, and all other things that are not accounted for by the CPU profiling.

## Usage

To use `tempo`, link the `github.com/profmagija/tempo/http` package into your program:

```go
import _ "github.com/profmagija/tempo/http"
```

This will register a HTTP handler at `/debug/tempo/wall` that will show the current timing information.

If your application is not already running an http server, you need to start one. Add `net/http` and `log` to your imports and the following code to your main function: 

```go
go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()
```

Then, you can use standard `pprof` tools to analyze the traces:

```
go tool pprof http://localhost:6060/debug/tempo/wall
```