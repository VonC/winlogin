package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"
	"unicode"

	"github.com/VonC/winlogin/version"
	"github.com/atotto/clipboard"
	"github.com/eiannone/keyboard"
	"github.com/vbauerster/mpb/v5"
	"github.com/vbauerster/mpb/v5/decor"
)

type app struct {
	sync.RWMutex
	name     string
	domain   string
	result   *res
	verbose  bool
	filename string
	// Users From Email: 'usersfe'
	usersfe users
}

// https://regex101.com/r/BQMZei/2
var re = regexp.MustCompile(`(?m)^\s+(?P<login>\S+)\s+(?P<email>(?P<firstname>[^@.]+)\.(?P<lastname>[^@.]+)@\S+)`)

type res struct {
	output string
	wusers users
}

type user struct {
	login     string
	lastname  string
	firstname string
	email     string
}

type users []*user

func (r *res) String() string {
	res := r.wusers.String()
	return res
}

func (u *user) String() string {
	return fmt.Sprintf("%s : %s %s (%s)", u.login, u.firstname, u.lastname, u.email)
}

func (us users) String() string {
	var csv = new(bytes.Buffer)
	var wcsv = tabwriter.NewWriter(csv, 0, 8, 2, '\t', 0)
	for _, u := range us {
		s := fmt.Sprintf("%s\t: %s\t%s\t(%s)\n", u.login, u.firstname, u.lastname, u.email)
		fmt.Fprint(wcsv, s)
	}
	wcsv.Flush()
	return csv.String()
}

func (a *app) hasOnlyOneUser() bool {
	return len(a.getRes().wusers) == 1
}

func main() {

	verbose := false
	filename := ""
	for _, f := range os.Args[1:] {
		fl := strings.ToLower(f)
		if f == "-V" || fl == "--version" || fl == "version" {
			fmt.Println(version.String())
			os.Exit(0)
		}
		if fl == "-v" || fl == "--verbose" {
			verbose = true
		} else {
			info, err := os.Stat(f)
			if err == nil && !info.IsDir() {
				filename = f
			}
		}
	}

	a := &app{verbose: verbose, filename: filename}
	if filename == "" {
		a.listenToKey()
	} else {
		a.parseFile()
	}
}

func (a *app) parseFile() {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	usersfe := make(users, 0)
	a.usersfe = usersfe
	usersfe = make(users, 0)
	file, err := os.Open(a.filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	d := a.getDomainMail()
	for scanner.Scan() {
		line := scanner.Text()
		usersfe = usersfe.extractEmails(line, d)
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
	nbFiles := len(usersfe)
	fmt.Printf("'%d' emails to process from email '%s'\n", nbFiles, a.filename)
	p := mpb.New(mpb.WithWidth(64))
	bar := p.Add(int64(nbFiles),
		nil,
		mpb.PrependDecorators(
			decor.Name(fmt.Sprintf("Email (%d): ", nbFiles)),
			decor.NewPercentage("%d"),
		),
	)
	for _, u := range usersfe {
		name := u.firstname
		if u.lastname != "" {
			name = name + " " + u.lastname
		}
		a.setName(name)
		a.lookupName(ctx, a.addToUsersfe)
		bar.Increment()
	}
	p.Wait()

	fmt.Printf("User from email '%s': '\n%s\n", a.filename, a.usersfe)
}

// https://regex101.com/r/uV73Fo/1
var reemails = regexp.MustCompile(`(?m)(?P<email>(?P<firstname>[a-zA-Z0-9\-_]+)(\.(?P<lastname>[a-zA-Z0-9\-_]+))?@(?P<domain>[a-zA-Z0-9\-_\.]+))`)

func (us users) extractEmails(line, udomain string) users {
	// Users From Email: 'usersfe'
	matches := reemails.FindAllStringSubmatch(line, -1)
	for _, match := range matches {
		if len(match) > 0 {
			firstname := match[reemails.SubexpIndex("firstname")]
			lastname := match[reemails.SubexpIndex("lastname")]
			email := match[reemails.SubexpIndex("email")]
			domain := match[reemails.SubexpIndex("domain")]
			if domain == udomain && !us.hasEmail(email) {
				//fmt.Printf("Extract from line email '%s':\n", email)
				//fmt.Printf("'%s': firstname '%s', lastname '%s', domain '%s'\n", email, firstname, lastname, domain)
				u := &user{
					firstname: firstname,
					lastname:  lastname,
					email:     email,
				}
				us = append(us, u)
			}
		}
	}
	return us
}

func (a *app) addToUsersfe(output string) {
	r := newRes(output)
	if len(r.wusers) == 1 {
		a.usersfe = append(a.usersfe, r.wusers[0])
	}
	if len(r.wusers) == 2 {
		n1 := r.wusers[0].login
		n2 := r.wusers[1].login
		if n2 == n1+"-adm" {
			a.usersfe = append(a.usersfe, r.wusers[0])
		}
	}
}

func (a *app) isValidDomain(aDomain string) bool {
	d := a.getDomainMail()
	return d == aDomain
}

func (us users) hasEmail(email string) bool {
	e := strings.ToLower(email)
	for _, u := range us {
		if strings.ToLower(u.email) == e {
			return true
		}
	}
	return false
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

func (a *app) setName(name string) {
	a.Lock()
	a.name = name
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
	r.wusers = make(users, 0)
	scanner := bufio.NewScanner(strings.NewReader(output))
	// log.Printf("output:'%s'", output)
	for scanner.Scan() {
		line := scanner.Text()
		matches := re.FindStringSubmatch(line)
		// log.Printf("line:'%s', matches '%+v'", line, matches)
		if len(matches) > 0 {
			login := matches[re.SubexpIndex("login")]
			firstname := matches[re.SubexpIndex("firstname")]
			lastname := matches[re.SubexpIndex("lastname")]
			email := matches[re.SubexpIndex("email")]
			u := &user{
				login:     login,
				firstname: firstname,
				lastname:  lastname,
				email:     email,
			}
			r.wusers = append(r.wusers, u)
		}
	}
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

func (a *app) logf(format string, v ...interface{}) {
	if a.verbose {
		log.Printf(format, v...)
	}
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
		a.logf("You pressed: rune %q ('%s'), key %X\r\n", event.Rune, string(event.Rune), event.Key)
		if event.Key == keyboard.KeyEsc {
			break
		}
		arune := event.Rune
		if unicode.IsLetter(arune) {
			a.logf("Add '%q/%X' to '%s'", event.Rune, event.Key, a.getName())
			n := a.getName()
			if n != "" {
				a.logf("call cancel1")
				cancel()
				<-ctx.Done()
				a.logf("Lookup with '%s' indeed CANCELLED\n", n)
			}
			ctx = context.Background()
			ctx, cancel = context.WithCancel(ctx)
			defer cancel()
			a.addToName(string(arune))
			a.searchForName(ctx)
			continue
		}
		if event.Key == 32 {
			a.logf("Add space to '%s'", a.getName())
			a.addToName(" ")
		} else {
			a.logf("Nope on '%d'", event.Key)
		}
	}
}

func (a *app) searchForName(ctx context.Context) {
	a.logf("Search for name '%s'\n", a.name)
	go a.lookupName(ctx, a.addToApp)
}

func (a *app) lookupName(ctx context.Context, f fres) {
	n := a.getName()

	// Start a process:
	// scmd := fmt.Sprintf("echo \"%s\">a&& ping 127.0.0.1 -n 8", n)
	scmd := a.getQueryFromName()
	a.logf("%s", scmd)

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
		a.logf("process finished successfully for n='%s'", n)
		f(bout.String())
	case <-ctx.Done():
		a.logf("Lookup with '%s' CANCELLED\n", n)
		if err := cmd.Process.Kill(); err != nil {
			log.Fatal(err)
		}
		a.setRes("")
	}
}

type fres func(string)

func (a *app) addToApp(output string) {
	a.setRes(output)
	log.Printf("Res for '%s':'\n%s'", a.getName(), a.getRes())
	if a.hasOnlyOneUser() {
		login := a.result.wusers[0].login
		log.Printf("Login '%s' copied to clipbord. Exiting.", login)
		errc := clipboard.WriteAll(login)
		if errc != nil {
			log.Fatalf("Login '%s' NOT copied to the clipboard because of '%+v'\n", login, errc)
		}
		os.Exit(0)
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
	if a.usersfe == nil {
		for i, elt := range elts {
			var buffer bytes.Buffer
			for _, rune := range elt {
				buffer.WriteRune(rune)
				buffer.WriteRune('*')
			}
			elts[i] = buffer.String()
		}
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
	//fmt.Printf("Query='%s'\n", query)
	return query
}
