package main

import "log"

func main() {
	if err := NewRuntime(NewApp()).Run(); err != nil {
		log.Fatal(err)
	}
}
