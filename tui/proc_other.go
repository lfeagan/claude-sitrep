//go:build !unix

package main

import "os/exec"

// confine does nothing where process groups are not available; sitrep itself needs a Unix shell.
func confine(*exec.Cmd) {}
