// +build !windows,!linux

package main

import "syscall"

func sysProcAddr() *syscall.SysProcAttr { return nil }
