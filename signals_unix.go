// +build !windows

package main

import (
	"os"
	"syscall"
)

func signals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM}
}
