package tempo

import (
	"net/http"
	"strconv"
	"time"

	"github.com/profmagija/tempo/internal"
)

func init() {
	http.HandleFunc("/debug/tempo/wall", Wall)
}

func Wall(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	secondsString := r.FormValue("seconds")
	seconds := 5

	if secondsString != "" {
		var err error
		seconds, err = strconv.Atoi(secondsString)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	err := internal.WriteTrace(ctx, time.Duration(seconds)*time.Second, w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
