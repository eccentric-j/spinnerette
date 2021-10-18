package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/cgi"
	"net/http/fcgi"
	"os"
	"path/filepath"
	"strings"

	janet "github.com/rushsteve1/spinnerette/bindings"
)

type Flags struct {
	Root   string
	Method string
	Port   int
	Socket string
}

var parsedFlags Flags

//go:embed libs/janet-html/src/janet-html.janet libs/spork/spork/*.janet libs/spin/*.janet
var embeddedLibs embed.FS

var fileMappings = map[string]string{
	"html":             "libs/janet-html/src/janet-html.janet",
	"spin":             "libs/spin/init.janet",
	"spin/response":    "libs/spin/response.janet",
	"spork":            "libs/spork/spork/init.janet",
	"spork/argparse":   "libs/spork/spork/argparse.janet",
	"spork/ev-utils":   "libs/spork/spork/ev-utils.janet",
	"spork/fmt":        "libs/spork/spork/fmt.janet",
	"spork/generators": "libs/spork/spork/generators.janet",
	"spork/http":       "libs/spork/spork/http.janet",
	"spork/init":       "libs/spork/spork/init.janet",
	"spork/misc":       "libs/spork/spork/misc.janet",
	"spork/msg":        "libs/spork/spork/msg.janet",
	"spork/netrepl":    "libs/spork/spork/netrepl.janet",
	"spork/path":       "libs/spork/spork/path.janet",
	"spork/regex":      "libs/spork/spork/regex.janet",
	"spork/rpc":        "libs/spork/spork/rpc.janet",
	"spork/temple":     "libs/spork/spork/temple.janet",
	"spork/test":       "libs/spork/spork/test.janet",
}

func main() {
	ParseFlags()
	parsedFlags.Method = strings.ToLower(parsedFlags.Method)

	janet.SetupEmbeds(embeddedLibs, fileMappings)

	handler := Handler{
		Addr: fmt.Sprintf("0.0.0.0:%d", parsedFlags.Port),
	}

	if parsedFlags.Method == "http" {
		log.Printf("Starting HTTP server on port %d", parsedFlags.Port)
		http.ListenAndServe(handler.Addr, handler)
	} else if parsedFlags.Method == "fastcgi" || parsedFlags.Method == "fcgi" {
		var listen net.Listener
		defer listen.Close()

		var err error
		if len(parsedFlags.Socket) > 0 {
			log.Printf("Starting FastCGI server on socket %s", parsedFlags.Socket)
			listen, err = net.Listen("unix", parsedFlags.Socket)
			if err != nil {
				log.Fatal(err)
			}
		} else {
			log.Printf("Starting FastCGI server on port %d", parsedFlags.Port)
			listen, err = net.Listen("tcp", handler.Addr)
			if err != nil {
				log.Fatal(err)
			}
		}

		fcgi.Serve(listen, handler)
	} else if parsedFlags.Method == "cgi" {
		log.Printf("Running as CGI program")
		cgi.Serve(handler)
	} else {
		log.Fatal("Unknown method")
	}
}

func ParseFlags() {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	flag.StringVar(&parsedFlags.Method, "method", "http", "The method that Spinnerette will listen on (HTTP, FastCGI, or CGI)")
	flag.StringVar(&parsedFlags.Root, "root", wd, "Webroot files will be found in")
	flag.IntVar(&parsedFlags.Port, "port", 9999, "Port to use for HTTP/FastCGI")
	flag.StringVar(&parsedFlags.Socket, "socket", "", "Socket to use for FastCGI (falls back to TCP with --port)")

	flag.Parse()
}

type Handler struct {
	Addr string
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := filepath.Join(parsedFlags.Root, r.URL.Path)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}

	switch filepath.Ext(path) {
	case ".janet":
		h.janetHandler(w, r, path)
	case ".temple":
		h.templeHandler(w, r, path)
	default:
		http.ServeFile(w, r, path)
	}
}

func (h Handler) janetHandler(w http.ResponseWriter, r *http.Request, path string) {
	janet.Init()
	defer janet.DeInit()

	env, err := janet.RequestEnv(r)
	if err != nil {
		http.Error(w, "Could not build request env", 500)
		log.Println(err)
		return
	}

	j, err := janet.EvalFilePath(path, env)
	if err != nil {
		http.Error(w, err.Error(), 500)
		log.Println(err.Error())
		return
	}

	janet.WriteResponse(j, w)
}

func (h Handler) templeHandler(w http.ResponseWriter, r *http.Request, path string) {
	janet.Init()
	defer janet.DeInit()

	env, err := janet.RequestEnv(r)
	if err != nil {
		http.Error(w, "Could not build request env", 500)
		log.Println(err)
		return
	}

	j, err := janet.RenderTemple(path, env)
	if err != nil {
		http.Error(w, err.Error(), 500)
		log.Println(err.Error())
		return
	}

	janet.WriteResponse(j, w)
}
