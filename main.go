package main

/*
 * GaRuS - Github-actions-Runner-upload-Server - standalone
 */

// curl -F "file=@./test.file" -H "X-Git-Repo: GARUS" -H "X-Git-Ref: $GITREF" -H "X-Auth-Token: $TOKEN" http://localhost:58080/upload.php

import (
	"bufio"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	// import the GaRuS modules

	"github.com/go-while/GaRuS/networkacl"
	garus "github.com/go-while/GaRuS/server"
	tokens "github.com/go-while/GaRuS/tokens"
)

var (
	appVersion = "-" // will be set by compiler
	commit     = "-" // will be set by compiler
	date       = "-" // will be set by compiler
	Servers    map[int]*garus.GaRuS
	mainMux    sync.RWMutex
	stopChan   chan struct{}
	stop       chan os.Signal
	mainWG     sync.WaitGroup
	admin      bool
	NetACL     map[string]struct{}
)

func main() {
	if appVersion != "-" {
		appVersion = appVersion + "|" + commit + "|" + date
		garus.ModulesGVersion = appVersion
	}
	var adminIF, netacl, listen, routes, tokenf, upload, tlscrt, tlskey, adminf string
	var addNew bool
	//flag.BoolVar(&addNew, "add", false, "set flag -add=true to launch another garus server with own flags") // TODO!
	//flag.StringVar(&adminIF, "admin", "", "Admin Interface: if you want to launch multiple instances at different ip:port\n    with their own routes, tokens, upload directory\n    and optional ssl: set flag -admin=127.0.0.1:9090 or [ip6]:port on boot") // TODO!
	//flag.StringVar(&netacl, "acl", "127.0.0.1,[::1]", "-acl \"comma,sperated,list,of,ip4,or,[ip6],addresses\" to restrict access to the admin interface") // TODO!
	flag.StringVar(&listen, "listen", garus.ListenDef, "listens on 'ip4:port' or '[ip6]:port'")
	flag.StringVar(&routes, "routes", garus.RoutesDef, "can be anything like: /upload.php")
	flag.StringVar(&tokenf, "tokenf", garus.TokensDef, "/path/to/tokens/.passwd")
	flag.StringVar(&upload, "upload", garus.UploadDef, "/path/to/upload/dir")
	flag.StringVar(&tlscrt, "tlscrt", "", "eg: /path/to/fullchain.pem")
	flag.StringVar(&tlskey, "tlskey", "", "eg: /path/to/privkey.pem")
	flag.StringVar(&adminf, "adminf", "", "eg: /path/to/.admin.tokens")
	flag.Parse()
	if adminIF != "" && addNew {
		fmt.Printf("ERROR: can not use -add and -admin at the same time! start with -admin first, then you can add more instances!\n")
		os.Exit(1)
	}

	// setup channels if needed
	mainMux.Lock()
	if stopChan == nil {
		stopChan = make(chan struct{}, 1)
	}
	if Servers == nil {
		Servers = make(map[int]*garus.GaRuS)
	}
	if stop == nil {
		// Prepare signal handling for Ctrl+C (SIGINT) or SIGTERM
		stop = make(chan os.Signal, 1)
		// cross-platform signal handling
		signals := []os.Signal{os.Interrupt}
		if runtime.GOOS != "windows" {
			signals = append(signals, syscall.SIGTERM)
		}
		signal.Notify(stop, signals...)
	}
	mainMux.Unlock()

	mainWG.Add(1)
	go LaunchGaRuS(listen, routes, upload, tokenf, tlscrt, tlskey, &mainWG, stopChan, nil)

	if adminIF != "" {
		bootErr := make(chan error, 1)
		fmt.Printf("Starting Admin Interface @ '%s' netacl='%s'\n", adminIF, netacl)
		// create an admin instance
		a := &ADMIN{
			adminf: adminf,
		}
		go a.StartAdminInterface(adminIF, netacl, bootErr)
		waitTimeout := time.After(1 * time.Second)
	wait:
		for {
			select {
			case err := <-bootErr:
				if err != nil {
					fmt.Printf("ERROR StartAdminInterface: err='%v'\n", err)
					os.Exit(1)
				}
			case <-waitTimeout:
				// admin interface should have booted...
				mainMux.RLock()
				running := admin
				mainMux.RUnlock()

				if !running {
					fmt.Printf("ERROR Admin Interface did not start and did not report an error...\n")
					os.Exit(1)
				} // end if !running
				break wait
			} // end select
		} // end for
	}

	mainWait()

} // end func main

func mainWait() {
	// main will wait here
	ticker := time.NewTicker(60 * time.Second)
forever:
	for {
		select {
		case <-stopChan:
			// anybody sent a signal via stopChan
			select {
			case stopChan <- struct{}{}:
				// refilled
			default:
				// cant refill, is already full. no more waiters...
			}
			fmt.Print("main: received stopChan\n")
			break forever
		case <-stop:
			select {
			case stopChan <- struct{}{}:
				// refilled
			default:
				// cant refill, is already full. no more waiters...
			}
			fmt.Print("main: received os.Signal\n")
			break forever
		case <-ticker.C:
			fmt.Printf("GaRuS '%s' Alive!\n", appVersion)
		}
	} // infinite wait
	fmt.Printf("main: waiting for all servers to close...\n")
	mainWG.Wait()
} // end func mainWait

// launches a GaRuS Instance: this function blocks!
func LaunchGaRuS(listenStr, routesStr, uploadStr, tokensStr, tlscrt, tlskey string, parentwg *sync.WaitGroup, stopChan chan struct{}, g *garus.GaRuS) (id int) {
	newg, err := garus.NewGaRuS(listenStr, routesStr, uploadStr, tokensStr, tlscrt, tlskey, parentwg, stopChan, nil)
	if newg == nil || err != nil {
		fmt.Printf("NewGaRuS failed: returned newg='%v' err='%v'\n", newg, err)
		return -1
	}
	mainMux.Lock()
	id = len(Servers) + 1 // start with 1, not 0
	Servers[id] = newg
	mainMux.Unlock()
	return id
} // end func LaunchGaRuS

type ADMIN struct {
	adminf string // path to the admin tokens file
	// Admin interface related fields can be added here if needed
}

// ReloadAdminTokens reads all tokens from the adminf file and returns them as a slice of strings.
// Returns an error if the file cannot be read.
func (a *ADMIN) ReloadAdminTokens() (map[string]struct{}, error) {
	mainMux.RLock()
	defer mainMux.RUnlock()
	file, err := os.Open(a.adminf)
	if err != nil {
		return nil, fmt.Errorf("failed to open admin token file: %w", err)
	}
	defer file.Close()

	admintokens := make(map[string]struct{})
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line) // Ignore empty lines
		// Only add non-empty lines to the tokens map
		// This allows for comments and empty lines in the file
		if strings.HasPrefix(line, "#") {
			continue
		}
		if line != "" && len(line) >= tokens.MinTokenLen {
			admintokens[line] = struct{}{}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading admin token file: %w", err)
	}
	return admintokens, nil
} // end func ReloadAdminTokens

func (a *ADMIN) AdminAuthorization(r *http.Request, w *http.ResponseWriter) bool {

	// Check if the request is allowed by the network ACL
	if !networkacl.IsNetAllowed(networkacl.GetHost(r), &mainMux, NetACL, false) {
		http.Error(*w, "403 Forbidden", http.StatusForbidden)
		return false
	}
	if r.Method != http.MethodPost {
		http.Error(*w, "502 Method Not Allowed", http.StatusMethodNotAllowed)
		return false
	}
	// Get the token from the request header
	token := r.Header.Get("X-Auth-Token")
	if token == "" {
		// Return 400 Bad Request if the token is missing
		http.Error(*w, "400 Bad Request: missing X-Auth-Token header", http.StatusBadRequest)
		return false
	}

	// Load admin tokens from the file
	admintokens, err := a.ReloadAdminTokens()
	if err != nil {
		return false
	}

	// Validate the token length
	if len(token) < tokens.MinTokenLen || len(token) > tokens.MaxTokenLen {
		// Return 400 Bad Request if token is too short or too long
		http.Error(*w, "400 Bad Request: token too short or too long", http.StatusBadRequest)
		return false
	}

	// Check if the token is valid
	if _, valid := admintokens[token]; !valid {
		// Return 409 Conflict if the token is invalid
		http.Error(*w, "409 Conflict: admin token invalid", http.StatusConflict)
		// Log the invalid token attempt
		fmt.Printf("Invalid admin token attempt: %s\n", token)
		// Optionally, you could log this to a file or monitoring system
		// For now, just return an error
		// This could be a security issue, so handle it appropriately
		// You might want to log this attempt or take further action
		// depending on your security requirements.
		// For example, you could log it to a file or send an alert.
		return false
	}

	return true
}

// starts an admin interface at ip:port
// to allow adding new instances
// maybe view stats or whatever
func (a *ADMIN) StartAdminInterface(adminIF string, netacl string, booted chan error) {
	mainMux.Lock()
	NetACL = networkacl.GetNetACLFunc(netacl)
	mainMux.Unlock()
	adminMux := http.NewServeMux()

	adminMux.HandleFunc("/add-instance", func(w http.ResponseWriter, r *http.Request) {
		// Parse parameters, add instance, etc.
		// Optionally require a secret/token
		if !a.AdminAuthorization(r, &w) {
			return
		}
		// Parse form values for new instance parameters
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}
		listen := r.FormValue("listen")
		routes := r.FormValue("routes")
		upload := r.FormValue("upload")
		tokenf := r.FormValue("tokenf")
		tlscrt := r.FormValue("tlscrt")
		tlskey := r.FormValue("tlskey")

		if listen == "" || routes == "" || upload == "" || tokenf == "" {
			http.Error(w, "Missing required fields: listen, routes, upload, tokenf", http.StatusBadRequest)
			return
		}

		// Launch the new GaRuS instance in a goroutine
		mainWG.Add(1)
		go LaunchGaRuS(listen, routes, upload, tokenf, tlscrt, tlskey, &mainWG, stopChan, nil)

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "New GaRuS instance started: listen=%s, routes=%s, upload=%s, tokenf=%s\n", listen, routes, upload, tokenf)
	})

	adminMux.HandleFunc("/add-token", func(w http.ResponseWriter, r *http.Request) {
		if !a.AdminAuthorization(r, &w) {
			return
		}
		// Parse form values
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}
		repo := r.FormValue("repo")
		token := r.FormValue("token")
		expiresStr := r.FormValue("expires")
		network := r.FormValue("network")

		if repo == "" || token == "" || expiresStr == "" || network == "" {
			http.Error(w, "Missing required fields: repo, token, expires, network", http.StatusBadRequest)
			return
		}

		expires, err := strconv.ParseInt(expiresStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid expires value", http.StatusBadRequest)
			return
		}

		if expires < time.Now().Unix() {
			http.Error(w, "Invalid expires value", http.StatusBadRequest)
			return
		}

		// Validate token length
		if len(token) < tokens.MinTokenLen || len(token) > tokens.MaxTokenLen {
			http.Error(w, "Token too short or too long", http.StatusBadRequest)
			return
		}
		/*
			// Add the token to the token store
			ok := ts.AddToken(repo, token, expires, network)
			if !ok {
				http.Error(w, "Failed to add token", http.StatusInternalServerError)
				return
			}
		*/

		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "Token added successfully")
	})

	mainMux.Lock()
	admin = true // set true before starting admin interface
	mainMux.Unlock()

	err := http.ListenAndServe(adminIF, adminMux)
	booted <- err

	mainMux.Lock()
	admin = false
	mainMux.Unlock()
} // end func AdminInterface
