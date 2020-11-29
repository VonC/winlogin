package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"unicode"

	"github.com/VonC/username/version"
	"github.com/eiannone/keyboard"
)

type app struct {
	sync.RWMutex
	name   string
	domain string
	result *res
}

type res struct {
	output string
}

func (r *res) String() string {
	return r.output
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

func (a *app) getRes() *res {
	a.RLock()
	r := a.result
	a.RUnlock()
	return r
}

func (a *app) setRes(output string) {
	r := newRes(output)
	a.Lock()
	a.result = r
	a.Unlock()
}

func newRes(output string) *res {
	r := &res{output: output}
	return r
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
		fmt.Printf("You pressed: rune %q ('%s'), key %X\r\n", event.Rune, string(event.Rune), event.Key)
		if event.Key == keyboard.KeyEsc {
			break
		}
		arune := event.Rune
		if unicode.IsLetter(arune) {
			log.Printf("Add '%q/%X' to '%s'", event.Rune, event.Key, a.getName())
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
			a.addToName(string(arune))
			a.searchForName(ctx)
			continue
		}
		if event.Key == 32 {
			log.Printf("Add space to '%s'", a.getName())
			a.addToName(" ")
		} else {
			log.Printf("Nope on '%d'", event.Key)
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
	// scmd := fmt.Sprintf("echo \"%s\">a&& ping 127.0.0.1 -n 8", n)
	scmd := a.getQueryFromName()
	log.Printf("%s", scmd)

	berr := &bytes.Buffer{}
	bout := &bytes.Buffer{}
	cmd := exec.Command("cmd", "/C", scmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.CmdLine = "cmd /C " + scmd
	cmd.Stderr = berr
	cmd.Stdout = bout

	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatalf("stdin error %s [%s]", err, berr.String())
	}
	err = stdin.Close()
	if err != nil {
		log.Fatalf("Close error ion stdin %s", err)
	}
	err = cmd.Start()
	if err != nil {
		log.Fatalf("start error %s [%s]", err, berr.String())
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
		a.setRes(bout.String())
		log.Printf("Res for '%s': '%s'", a.getName(), "res") // a.getRes())
	case <-ctx.Done():
		log.Printf("Lookup with '%s' CANCELLED\n", n)
		if err := cmd.Process.Kill(); err != nil {
			log.Fatal(err)
		}
		a.setRes("")
	}
}

func (a *app) getQueryFromName() string {
	n := a.getName()
	if n == "" {
		return ""
	}
	elts := strings.Split(strings.TrimSpace(n), " ")
	if len(elts) != 1 && len(elts) != 2 {
		log.Fatalf("Invalid split on name '%s'", n)
	}
	query := ""
	for i, elt := range elts {
		var buffer bytes.Buffer
		for _, rune := range elt {
			buffer.WriteRune(rune)
			buffer.WriteRune('*')
		}
		elts[i] = buffer.String()
	}
	filters := make([]string, 0)
	if len(elts) == 1 {
		filters = append(filters, elts[0])
	} else {
		f1 := fmt.Sprintf("%s.%s", elts[0], elts[1])
		f2 := fmt.Sprintf("%s.%s", elts[1], elts[0])
		filters = append(filters, f1, f2)
	}
	for _, f := range filters {
		query = query + fmt.Sprintf("(&(objectCategory=Person)(objectClass=User)(mail=%s@%s))", f, a.getDomainMail())
	}
	if len(elts) == 2 {
		query = fmt.Sprintf("(|%s)", query)
	}
	query = fmt.Sprintf("DSQUERY * domainroot -filter \"%s\" -attr sAMAccountName mail", query)
	return query
}
