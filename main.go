package main

import (
	"fmt"
	"runtime"
	"time"
)

func main()  {
	fmt.Println(time.Time{})
	fmt.Println("goroot:",runtime.GOROOT())
}