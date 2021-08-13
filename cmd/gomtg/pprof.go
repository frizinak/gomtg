// +build pprof

package main

import (
	"net/http"
	_ "net/http/pprof"
)

func init() {
	go http.ListenAndServe(":6060", nil)
}
