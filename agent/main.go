package main

import (
	"syscall"

	"agent/cmd"
)

func main() {
	syscall.Umask(0007)

	cmd.Execute()
}
