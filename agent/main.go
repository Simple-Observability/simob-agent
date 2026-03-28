package main

import (
	"agent/cmd"
	"agent/internal/bootstrap"
)

func main() {
	bootstrap.SetUmask()

	cmd.Execute()
}
