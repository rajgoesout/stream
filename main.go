package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"html"
	"log"
	"net/http"

	"os"
	"os/exec"

	guuid "github.com/google/uuid"
)

type StreamResponse struct {
	Id  string
	Url string
}

func main() {
	path, err := os.Getwd()
	if err != nil {
		log.Println(err)
	}
	fmt.Println(path)

	// Don't allow accessing /streams/ route
	// (only allow `/streams/:uuid/` or `/streams/:uuid/:uuid.m3u8`)
	http.HandleFunc("/streams/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/streams/" {
			println(path + " now " + r.URL.Path)
			http.ServeFile(w, r, r.URL.Path[1:])
		} else {
			println("no id provided!")
			http.NotFound(w, r)
			return
		}
	})

	http.HandleFunc("/submit/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/submit/" {
			http.NotFound(w, r)
			return
		}

		if r.Method == "GET" {
			fmt.Fprintf(w, "GET, %q", html.EscapeString(r.URL.Path))
		} else if r.Method == "POST" {
			println(r.FormValue("input"))
			id := guuid.New()
			outputfilename := id.String() + ".m3u8"
			cmd1 := exec.Command("mkdir", "streams/"+id.String())
			cmd1.Output()
			outputPath := "./streams/" + id.String() + "/" + outputfilename
			urlinput := r.FormValue("input")
			resolution, e := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=width,height", "-of", "default=nw=1", urlinput).Output()
			if e != nil {
				log.Fatal(e)
			}
			println("resolution", resolution)
			width := strings.Split(strings.Split(string(resolution), "\n")[0], "=")[1]
			height := strings.Split(strings.Split(string(resolution), "\n")[1], "=")[1]
			reso := width + "x" + height
			// using same resolution as the original video
			fmt.Printf("The resolution is %s\n", reso)

			cmd := exec.Command("ffmpeg", "-i", urlinput, "-profile:v", "baseline", "-level", "3.0", "-s", reso, "-start_number", "0", "-hls_time", "10", "-hls_list_size", "0", "-f", "hls", outputPath)
			stdout, err := cmd.Output()

			if err != nil {
				println(err.Error())
				if err.Error() == "exit status 1" {
					fmt.Fprintf(w, "Please use a valid url (which returns a mp4 file)")
					println("Please use a valid url (which returns a mp4 file)")
				}
				return
			}
			hostURL := "http://" + r.Host + "/streams/"
			streamResponse := StreamResponse{id.String(), hostURL + id.String() + "/" + id.String() + ".m3u8"}
			js, err2 := json.Marshal(streamResponse)
			if err2 != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.Write(js)

			println(string(stdout))
		} else {
			http.Error(w, "Invalid request method.", 405)
		}

	})

	http.ListenAndServe(":80", nil)
}
