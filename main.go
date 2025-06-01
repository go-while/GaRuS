package main

/*
 * GaRuS - Github-actions-Runner-upload-Server - standalone
 */

// curl -F "file=@./test.file" -H "X-Git-Repo: GARUS" -H "X-Git-Ref: $GITREF" -H "X-Auth-Token: $TOKEN" http://localhost:58080/upload.php

import (
	"flag"
	"fmt"
	"github.com/go-while/GaRuS/networkacl"
	"github.com/go-while/GaRuS/server"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"
)

var (
	appVersion = "-" // will be set by compiler
	commit = "-" // will be set by compiler
	date = "-" // will be set by compiler
	Servers  map[int]*garus.GaRuS
	mainMux  sync.RWMutex
	stopChan chan struct{}
	stop     chan os.Signal
	mainWG   sync.WaitGroup
	admin    bool
	NetACL   map[string]struct{}
)

func main() {
	if appVersion != "-" {
		appVersion = appVersion+"|"+commit+"|"+date
		garus.ModulesGVersion = appVersion
	}
	var adminIF, netacl, listen, routes, tokenf, upload, tlscrt, tlskey string
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

	if adminIF != "" {
		bootErr := make(chan error, 1)
		fmt.Printf("Starting Admin Interface @ '%s' netacl='%s'\n", adminIF, netacl)
		go StartAdminInterface(adminIF, netacl, bootErr)
		waitTimeout := time.After(1 * time.Second)
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
				break
			} // end select
		} // end for
	}

	mainWG.Add(1)
	go LaunchGaRuS(listen, routes, upload, tokenf, tlscrt, tlskey, &mainWG, stopChan, nil)

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
			fmt.Printf("GaRuS '%s' Alive!\n")
		}
	} // infinite wait
	fmt.Printf("main: waiting for all servers to close...\n")
	mainWG.Wait()
} // end func mainWait


// launches a GaRuS Instance: this function blocks!
func LaunchGaRuS(listenStr, routesStr, uploadStr, tokensStr, tlscrt, tlskey string, parentwg *sync.WaitGroup, stopChan chan struct{}, g *garus.GaRuS) {
	g, err := garus.NewGaRuS(listenStr, routesStr, uploadStr, tokensStr, tlscrt, tlskey, parentwg, stopChan, nil)
	if g == nil || err != nil {
		fmt.Printf("NewGaRuS failed: returned g='%v' err='%v'\n", g, err)
	}
} // end func LaunchGaRuS

// starts an admin interface at ip:port
// to allow adding new instances
// maybe view stats or whatever
func StartAdminInterface(adminIF string, netacl string, booted chan error) {
	mainMux.Lock()
	NetACL = networkacl.GetNetACLFunc(netacl)
	mainMux.Unlock()
	adminMux := http.NewServeMux()

	adminMux.HandleFunc("/add-instance", func(w http.ResponseWriter, r *http.Request) {
		// Parse parameters, add instance, etc.
		// Optionally require a secret/token
		if !networkacl.IsNetAllowed(networkacl.GetHost(r), &mainMux, NetACL, false) {
			http.Error(w, "403", http.StatusForbidden)
			return
		}
		// TODO!!!
	})

	adminMux.HandleFunc("/add-token", func(w http.ResponseWriter, r *http.Request) {
		if !networkacl.IsNetAllowed(networkacl.GetHost(r), &mainMux, NetACL, false) {
			http.Error(w, "403", http.StatusForbidden)
			return
		}
		// TODO!!!
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
