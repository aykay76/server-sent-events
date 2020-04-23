package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// declare some metrics here for instrumentation
var (
	opsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "sse_processed_ops_total",
		Help: "The total number of processed events",
	})
)

func writeOutput(w http.ResponseWriter, input io.ReadCloser) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Important to make it work in browsers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	rd := bufio.NewReader(input)
	var str string
	var err error

	str, err = rd.ReadString('\n')
	if err != nil {
		fmt.Println("Closing")
		input.Close()
	}
	for err == nil {
		str, err = rd.ReadString('\n')
		if err == nil {
			w.Write([]byte("data: "))
			w.Write([]byte(str))
			w.Write([]byte("\n"))
			flusher.Flush()
		} else {
			input.Close()
		}
	}

	fmt.Println("Done")
}

func main() {
	// TODO: make modular so I can reuse in other projects
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// increment metric for number of requests
		opsProcessed.Inc()

		// TODO: make my root directory configurable
		filename := "./content" + r.URL.Path

		// add default document
		if strings.HasSuffix(filename, "/") {
			filename = filename + "index.html"
		}

		// TODO: maybe improve the logging a bit ;)
		fmt.Println(filename)

		// open the file ready for some additional work
		file, err := os.Open(filename)
		if err != nil {
			// handle the error and return
		}
		defer file.Close()

		fi, err := file.Stat()
		if err != nil {
			// handle the error and return
		}
		if fi.IsDir() {
			// it's a directory
			w.WriteHeader(http.StatusForbidden)
		} else {
			body, err := ioutil.ReadFile(filename)
			if err != nil {
				fmt.Println(err)
			}

			// use built-in MIME dictionary for common types
			contentType := mime.TypeByExtension(filepath.Ext(filename))
			w.Header().Set("Content-Type", contentType)

			// handle any SSI or special stuff
			if strings.HasSuffix(filename, ".html") {
				// convert to string and do some basic SSI
				bodyString := string(body)

				idx := strings.Index(bodyString, "<!--#include file=")
				for idx != -1 {
					idx2 := strings.Index(bodyString, "-->")
					subfile := bodyString[idx+19 : idx2-1]

					subfileContent, _ := ioutil.ReadFile("./content" + subfile)

					newBodyString := bodyString[0:idx] + string(subfileContent) + bodyString[idx2+3:len(bodyString)]
					bodyString = newBodyString
					idx = strings.Index(bodyString, "<!--#include file=")
				}

				body = []byte(bodyString)
			}

			w.Write(body)
		}
	})

	http.HandleFunc("/exec/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("Got call to execute something")
		if r.Method == "POST" {
			r.ParseForm()
		}

		cmd := exec.Command("cat", "/etc/passwd")
		rPipe, wPipe, err := os.Pipe()
		if err != nil {
			log.Fatal(err)
		}
		cmd.Stdout = wPipe
		cmd.Stderr = wPipe
		if err := cmd.Start(); err != nil {
			log.Fatal(err)
		}
		writeOutput(w, rPipe)
		cmd.Wait()
		wPipe.Close()
	})

	http.Handle("/metrics", promhttp.Handler())

	fmt.Println("Listening on http://localhost:8080")
	err := http.ListenAndServe(":8080", nil) // this doesn't handle SSI but worth knowing about: http.FileServer(http.Dir("./content")))
	if err != nil {
		fmt.Println(err)
	}
}
