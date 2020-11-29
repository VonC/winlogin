package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"unicode"

	"github.com/VonC/username/version"
	"github.com/eiannone/keyboard"
)

type app struct {
	sync.RWMutex
	name   string
	domain string
}

func main() {

	for _, f := range os.Args[1:] {
		fl := strings.ToLower(f)
		if fl == "-v" || fl == "--version" || fl == "version" {
			fmt.Println(version.String())
			os.Exit(0)
		}
	}

	a := &app{}
	a.listenToKey()
}

func (a *app) getName() string {
	a.RLock()
	n := a.name
	a.RUnlock()
	return n
}

func (a *app) addToName(c string) {
	a.Lock()
	if c != " " || !strings.Contains(a.name, " ") {
		a.name = a.name + c
	}
	a.Unlock()
}

func (a *app) getDomainMail() string {
	a.RLock()
	d := a.domain
	a.RUnlock()
	if d == "" {
		d = os.Getenv("USERMAIL")
		ds := strings.Split(d, "@")
		if len(ds) != 2 {
			log.Fatalf("Invalid USERMAIL domain mail '%s'", d)
		}
		d = ds[1]
		a.Lock()
		a.domain = d
		a.Unlock()
	}
	return d
}

func (a *app) listenToKey() {
	keysEvents, err := keyboard.GetKeys(10)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = keyboard.Close()
	}()

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	fmt.Printf("Look for login of user emails '@%s'\n", a.getDomainMail())
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
		rune := event.Rune
		if unicode.IsLetter(rune) {
			n := a.getName()
			if n != "" {
				fmt.Println("call cancel1")
				cancel()
				<-ctx.Done()
				log.Printf("Lookup with '%s' indeed CANCELLED\n", n)
			}
			ctx = context.Background()
			ctx, cancel = context.WithCancel(ctx)
			defer cancel()
			a.addToName(string(rune))
			a.searchForName(ctx)
		}
		if unicode.IsSpace(rune) {
			a.addToName(" ")
		}
	}
}

func (a *app) searchForName(ctx context.Context) {
	fmt.Printf("Search for name '%s'\n", a.name)
	go a.lookupName(ctx)
}

func (a *app) lookupName(ctx context.Context) {
	n := a.getName()

	// Start a process:
	scmd := fmt.Sprintf("echo \"%s\">a&& ping 127.0.0.1 -n 8", n)
	cmd := exec.Command("cmd", "/K", scmd)
	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	// Wait for the process to finish or kill it after a timeout (whichever happens first):
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case err := <-done:
		if err != nil {
			log.Fatalf("process finished with error = %v for n='%s'", err, n)
		}
		log.Printf("process finished successfully for n='%s'", n)
	case <-ctx.Done():
		log.Printf("Lookup with '%s' CANCELLED\n", n)
		if err := cmd.Process.Kill(); err != nil {
			log.Fatal(err)
		}
	}
}
