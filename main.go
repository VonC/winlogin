package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/VonC/username/version"
	"github.com/eiannone/keyboard"
)

func main() {

	for _, f := range os.Args[1:] {
		fl := strings.ToLower(f)
		if fl == "-v" || fl == "--version" || fl == "version" {
			fmt.Println(version.String())
			os.Exit(0)
		}
	}

	keysEvents, err := keyboard.GetKeys(10)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = keyboard.Close()
	}()

	fmt.Println("Press ESC to quit")
	for {
		event := <-keysEvents
		if event.Err != nil {
			panic(event.Err)
		}
		fmt.Printf("You pressed: rune %q, key %X\r\n", event.Rune, event.Key)
		if event.Key == keyboard.KeyEsc {
			break
		}
	}
}
