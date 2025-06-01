package garus

/*
 * GaRuS - Github-actions-Runner-upload-Server - importable module
 */

// curl -F "file=@./test.file" -H "X-Git-Repo: GaRuS" -H "X-Git-Ref: $GITREF" -H "X-Auth-Token: $TOKEN" http://localhost:58080/upload.php

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
	"github.com/go-while/GaRuS/networkacl"
	"github.com/go-while/GaRuS/tokens"
)

var modulesGVersion = "-" // will be set by git on compile time
const DefaultMemoryLimit = 10 * 1024 * 1024
const DefaultBufferSize = 10 * 1024 * 1024
const DefaultTLSkey = "privkey.pem"
const DefaultTLScrt = "fullchain.pem"

const ListenDef = "[::]:58080"
const RoutesDef = "/upload.php"
const TokensDef = "./.passwd"
const UploadDef = "/tmp/test/garus/uploads"

const DefaultHttpIdleTimeout = 5 // seconds (close idle conns after N seconds)
const DefaultHttpReadTimeout = 60 // seconds (affects max upload time to server)
const DefaultHttpSendTimeout = 60 // seconds (affects max download time from server)

// these variables can be set once via 'garus.* = X' after import, before calling garus.NewGarus(...)
var (
	HttpIdleTimeout = DefaultHttpIdleTimeout
	HttpReadTimeout = DefaultHttpReadTimeout
	HttpSendTimeout = DefaultHttpSendTimeout
)

type GaRuS struct {
	mux sync.RWMutex
	ts  *tokens.TokenStore
	upServers []*UpServer
}

type UpServer struct {
	mux sync.RWMutex
	TLS    bool
	SrvTCP *HttpServer
	SrvTLS *HttpsServer
	Listen string // ip:port
	Routes string // /upload.php
	TokenF string // path to file
	Upload string // path to dir
	stopwg *sync.WaitGroup
	stopCh chan struct{}
}

// Server interface with Start and Stop
type Server interface {
	Start()
	Stop(ctx context.Context) error
}

// HttpServer struct for graceful start/stop
type HttpServer struct {
	httpServer *http.Server
	srvstr     string // ip:port
	wgsrv      sync.WaitGroup
	stopChan   chan struct{}
}

// HttpsServer struct for graceful start/stop
type HttpsServer struct {
	httpServer *http.Server
	srvstr     string // ip:port
	tlscrt     string
	tlskey     string
	stopChan   chan struct{}
	wgsrv      sync.WaitGroup
}

// Start for HttpServer
func (s *HttpServer) Start() {
	s.wgsrv.Add(1)
	go func() {
		defer s.wgsrv.Done()
		// blocks after this line!
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("ERROR ListenAndServe: err='%v'\n", err)
		}
		// releases when server stops listening
		/*
		select {
			case s.stopChan <- struct{}{}:
				fmt.Printf("TCP server: s.stopChan sent!")
			default:
				fmt.Printf("TCP server: s.stopChan is full, ignore!")
		}
		*/
	}()
}

// Stop for HttpServer
func (s *HttpServer) Stop(ctx context.Context) error {
	fmt.Print("HttpServer.Stop() ... wait")
	err := s.httpServer.Shutdown(ctx)
	s.wgsrv.Wait()
	fmt.Printf("HttpServer.Stop() quit: err='%v'\n", err)
	return err
}


// Start for HttpsServer
func (s *HttpsServer) Start() {
	s.wgsrv.Add(1)
	go func() {
		defer s.wgsrv.Done()
		// blocks after this line!
		if err := s.httpServer.ListenAndServeTLS(s.tlscrt, s.tlskey); err != nil && err != http.ErrServerClosed {
			fmt.Printf("ERROR: HTTPS server ListenAndServeTLS err='%v'\n", err)
		}
		// releases when server stops listening
		/*
		fmt.Printf("TLS server: s.stopChan sending")
		select {
			case s.stopChan <- struct{}{}:
				fmt.Printf("TLS server: s.stopChan sent!")
			default:
				fmt.Printf("TLS server: s.stopChan is full, ignore!")
		}
		*/
	}()
}

// Stop for HttpsServer
func (s *HttpsServer) Stop(ctx context.Context) error {
	fmt.Print("HttpsServer.Stop() ... wait\n")
	err := s.httpServer.Shutdown(ctx)
	s.wgsrv.Wait()
	fmt.Printf("HttpsServer.Stop() quit: err='%v'\n", err)
	return err
}


// Factory for HttpServer
func NewHttpServer(addr string, handler http.Handler, stopChan chan struct{}) *HttpServer {
	return &HttpServer{
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      handler,
			IdleTimeout:  time.Duration(HttpIdleTimeout) * time.Second,
			ReadTimeout:  time.Duration(HttpReadTimeout) * time.Second,
			WriteTimeout: time.Duration(HttpSendTimeout) * time.Second,
		},
		stopChan: stopChan,
	}
} // end func NewHttpServer

// Factory for HttpsServer
func NewHttpsServer(addr string, handler http.Handler, tlscrt, tlskey string, stopChan chan struct{}) *HttpsServer {
	return &HttpsServer{
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      handler,
			IdleTimeout:  time.Duration(HttpIdleTimeout) * time.Second,
			ReadTimeout:  time.Duration(HttpReadTimeout) * time.Second,
			WriteTimeout: time.Duration(HttpSendTimeout) * time.Second,
		},
		tlscrt: tlscrt,
		tlskey: tlskey,
		stopChan: stopChan,
	}
}

// function to start a new GaRuS instance
func NewGaRuS(listenStr, routesStr, uploadStr, tokensStr, tlscrt, tlskey string, parentwg *sync.WaitGroup, stopChan chan struct{}, g *GaRuS) (*GaRuS, error) {
	// provide "g" if you already have create a GaRuS instance and want to boot another ip:port!

	if tokensStr == "" || listenStr == "" || routesStr == "" || uploadStr == "" {
		return nil, fmt.Errorf("ERROR in NewGarus_: missing inputs?! tokens='%s' listen='%s' route='%s' updir='%s'\n", tokensStr, listenStr, routesStr, uploadStr)
	}

	if err := os.MkdirAll(uploadStr, 0755); err != nil {
		return nil, fmt.Errorf("Failed to create upload dir: %v\n", err)
	}

	// Prepare token store and handler
	ts := tokens.NewTokenStore(tokensStr)
	// prepare webserver
	mux := http.NewServeMux()
	mux.HandleFunc(routesStr, uploadHandler(uploadStr, ts))
	// check for tls
	var srvTCP *HttpServer
	var srvTLS *HttpsServer
	var TLS bool
	if tlscrt == "" && tlskey == "" {
		srvTCP = NewHttpServer(listenStr, mux, stopChan)
	} else {
		srvTLS = NewHttpsServer(listenStr, mux, tlscrt, tlskey, stopChan)
		TLS = true
	}

	// the new GaRuS instance  in "g"
	// which launces on ip:port
	// and uses the route to accept uploads to upDir
	if g == nil {
		g = &GaRuS{
			ts: ts,
		}
	}

	// pointer to the new UpServer which serves the routes
	newSrv := &UpServer{
		TLS:    TLS,
		SrvTCP: srvTCP,
		SrvTLS: srvTLS,
		TokenF: tokensStr,
		Listen: listenStr,
		Routes: routesStr,
		Upload: uploadStr,
		stopwg: parentwg,
		stopCh: stopChan,
	}

	g.mux.Lock()
	g.upServers = append(g.upServers, newSrv)
	instances := len(g.upServers)
	g.mux.Unlock()

	if modulesGVersion == "-" {
		modulesGVersion = fmt.Sprintf("noBuildVersion! ::: test-run: started=%d :::\n", time.Now().Unix())
	}

	fmt.Printf("Starting GaRuS Version: [%s] | TLS=%t @ '%s' tokf='%s' route='%s' uploadDir='%s' | Instances=%d\n", modulesGVersion, newSrv.TLS, newSrv.Listen, newSrv.TokenF, newSrv.Routes, newSrv.Upload, instances)

	go func(mux http.Handler, thisUpServer *UpServer) {
		defer thisUpServer.stopwg.Done()
		// Create and start our Http(s)Server
		switch thisUpServer.TLS {
		case true:
			thisUpServer.SrvTLS.Start()
		default:
			thisUpServer.SrvTCP.Start()
		}


		// wait for signals to stop
		<-thisUpServer.stopCh
		thisUpServer.stopCh <- struct{}{} // re-fill to stop any others
		fmt.Printf("Received shutdown signal, stopping server...")

		// Stop the server gracefully (10 sec timeout)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		switch thisUpServer.TLS {
		case true:
			if err := thisUpServer.SrvTLS.Stop(ctx); err != nil {
				fmt.Printf("TCP HTTP Server shutdown error: %v\n", err)
			}
		default:
			if err := thisUpServer.SrvTCP.Stop(ctx); err != nil {
				fmt.Printf("TLS HTTP Server shutdown error: %v\n", err)
			}
		}
		fmt.Println("Server stopped gracefully.")
	}(mux, newSrv)
	return g, nil
} // end func NewGaRuS

func uploadHandler(updir string, ts *tokens.TokenStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only accept POST
		if r.Method != http.MethodPost {
			http.Error(w, "i", http.StatusMethodNotAllowed)
			return
		}
		host := networkacl.GetHost(r)
		repo := r.Header.Get("X-Git-Repo") // go-while/GaRuS
		gitref := r.Header.Get("X-Git-Ref") // the branch
		gitsha7 := r.Header.Get("X-Git-SHA7") // the shorted commit hash
		compiler := r.Header.Get("X-Git-Comp") // info about who compiled it "SHR=self-hosted runner", "GOR=GoReleaser", ..."
		token := r.Header.Get("X-Auth-Token")
		if len(compiler) == 0 {
			compiler = "undef"
		}
		if repo == "" || len(gitsha7) != 7 || token == "" || len(token) < tokens.MinTokenLen || gitref == "" || len(gitref) <= 6 || len(gitref) > 64 {
			fmt.Printf("Missing fields! repo='%s' token=%d gitref='%s' gitsha7='%s' compiler='%s' host='%s'\n", repo, len(token), gitref, gitsha7, compiler, host)
			http.Error(w, "d", http.StatusUnauthorized)
			return
		}

		auth := ts.Auth(repo, token, r, false)
		if !auth.Valid {
			fmt.Printf("Invalid access! repo='%s' gitref='%s' reason='%s' host='%s'\n", repo, gitref, auth.Reason, host)
			http.Error(w, "d", http.StatusUnauthorized)
			return
		}

		// Parse multipart form
		err := r.ParseMultipartForm(DefaultMemoryLimit)
		if err != nil {
			http.Error(w, "m", http.StatusBadRequest)
			fmt.Printf("Could not parse multipart form: host='%s' err='%v'\n", host, err)
			return
		}

		// Get file from posted form-data
		data, handler, err := r.FormFile("file")
		if err != nil {
			fmt.Printf("No file uploaded: host='%s' err='%v'\n", host, err)
			http.Error(w, "e", http.StatusBadRequest)
			return
		}
		defer data.Close()

		filename := filepath.Base(handler.Filename)
		dstDir := filepath.Join(updir, repo, gitref, gitsha7, compiler)
		dstFile := filepath.Join(dstDir, filename)

		if FileExists(dstFile) {
			fmt.Printf("Repo: '%s' | upload denied! exists='%s' host='%s'\n", repo, dstFile, host)
			http.Error(w, "#", http.StatusFound)
			return
		}

		if !DirExists(dstDir) {
			if err := os.MkdirAll(dstDir, 0755); err != nil {
				fmt.Print(err)
				http.Error(w, "!", http.StatusInternalServerError)
				return
			}
		}
		tmpfilename := fmt.Sprintf(".%s.t%d.r%d.tmp", filename, time.Now().Unix(), rand.Intn(999999))
		dstTmp := filepath.Join(dstDir, tmpfilename)

		file, err := os.OpenFile(dstTmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Print(err)
			http.Error(w, "p", http.StatusInternalServerError)
			return
		}

		bufWriter := bufio.NewWriterSize(file, DefaultBufferSize)

		if _, err := io.Copy(bufWriter, data); err != nil {
			fmt.Print(err)
			http.Error(w, "s", http.StatusInternalServerError)
			return
		}
		bufWriter.Flush()
		file.Close()

		if err := os.Rename(dstTmp, dstFile); err != nil {
			fmt.Print(err)
			http.Error(w, "x", http.StatusInternalServerError)
			return
		}

		fmt.Printf("Repo: '%s' | New file='%s'\n", repo, dstFile)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("+"))
	}
}

// DirExists returns true if the given path exists and is a directory.
func DirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// FileExists returns true if the given path exists and is a regular file.
func FileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}
