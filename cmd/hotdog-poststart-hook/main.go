package main

import "errors"

func main() {
	if err := _main(); err != nil {
		panic(err)
	}
}

func _main() error {
	return errors.New("poststart")
}
