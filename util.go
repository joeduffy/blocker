package main

import (
	. "log"
	"os"
)

var stdout *Logger
var stderr *Logger

func init() {
	stdout = New(os.Stdout, "", Ldate|Ltime)
	stderr = New(os.Stderr, "error: ", Ldate|Ltime)
}

func log(format string, a ...interface{}) {
	stdout.Printf(format, a...)
}

func logError(format string, a ...interface{}) {
	stderr.Printf(format, a...)
}
