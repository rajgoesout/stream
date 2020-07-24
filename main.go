package main

import (
	"context"
	"encoding/json"
	"fmt"
	// "io"
	// "math/rand"
	"strings"
	"time"

	"html"
	"net/http"

	"os"
	"os/exec"

	// log "github.com/sirupsen/logrus"

	logging "github.com/ipfs/go-log/v2"

	guuid "github.com/google/uuid"
	"github.com/multiformats/go-multiaddr"
	"github.com/textileio/powergate/api/client"

	"github.com/textileio/powergate/ffs"
	"github.com/textileio/powergate/health"
	"google.golang.org/grpc"
)

// StreamResponse resp
type StreamResponse struct {
	ID  string
	URL string
}

// StreamResponseABR resp
type StreamResponseABR struct {
	ID       string
	URL360p  string
	URL480p  string
	URL720p  string
	URL1080p string
}

// Setup for powergate client
type Setup struct {
	LotusAddr    multiaddr.Multiaddr
	MinerAddr    string
	SampleSize   int64
	MaxParallel  int
	TotalSamples int
	RandSeed     int
}

var (
	log = logging.Logger("runner")
)

var (
	lotusAddr = multiaddr.StringCast("/ip4/0.0.0.0/tcp/5002")
)

// Run pow client
func Run(ctx context.Context, setup Setup) error {
	c, err := client.NewClient(setup.LotusAddr, grpc.WithInsecure(), grpc.WithPerRPCCredentials(client.TokenAuth{}))
	defer func() {
		if err := c.Close(); err != nil {
			log.Errorf("closing powergate client: %s", err)
		}
	}()
	if err != nil {
		return fmt.Errorf("creating client: %s", err)
	}

	if err := sanityCheck(ctx, c); err != nil {
		return fmt.Errorf("sanity check with client: %s", err)
	}

	if err := runSetup(ctx, c, setup); err != nil {
		return fmt.Errorf("running test setup: %s", err)
	}

	return nil
}

func sanityCheck(ctx context.Context, c *client.Client) error {
	s, _, err := c.Health.Check(ctx)
	if err != nil {
		return fmt.Errorf("health check call: %s", err)
	}
	if s != health.Ok {
		return fmt.Errorf("reported health check not Ok: %s", s)
	}
	return nil
}

func runSetup(ctx context.Context, c *client.Client, setup Setup) error {
	_, tok, err := c.FFS.Create(ctx)
	if err != nil {
		return fmt.Errorf("creating ffs instance: %s", err)
	}
	fmt.Println("ffs tok", tok)
	ctx = context.WithValue(ctx, client.AuthKey, tok)
	info, err := c.FFS.Info(ctx)
	if err != nil {
		return fmt.Errorf("getting instance info: %s", err)
	}
	fmt.Println("ffs info", info)
	addr := info.Balances[0].Addr
	time.Sleep(time.Second * 5)

	chLimit := make(chan struct{}, setup.MaxParallel)
	chErr := make(chan error, setup.TotalSamples)
	for i := 0; i < setup.TotalSamples; i++ {
		chLimit <- struct{}{}
		go func(i int) {
			defer func() { <-chLimit }()
			if err := run(ctx, c, i, setup.RandSeed+i, setup.SampleSize, addr, setup.MinerAddr); err != nil {
				chErr <- fmt.Errorf("failed run %d: %s", i, err)
			}
		}(i)
	}
	for i := 0; i < setup.MaxParallel; i++ {
		chLimit <- struct{}{}
	}
	close(chErr)
	for err := range chErr {
		return fmt.Errorf("sample run errored: %s", err)
	}
	return nil
}

func run(ctx context.Context, c *client.Client, id int, seed int, size int64, addr string, minerAddr string) error {
	log.Infof("[%d] Executing run...", id)
	defer log.Infof("[%d] Done", id)
	// ra := rand.New(rand.NewSource(int64(seed)))
	// lr := io.LimitReader(ra, size)
	ior, _ := os.Open("./small.mp4")
	// ior, _ := os.Open("./folder")
	// cmda := exec.Command("ffmpeg", "-i", urlinput, "-profile:v", "baseline", "-level", "3.0", "-s", "640x360", "-start_number", "0", "-hls_time", "10", "-hls_list_size", "0", "-f", "hls", outputPath+"/360p.m3u8")
	// 		cmda.Output()

	log.Infof("[%d] Adding to hot layer...", id)
	fmt.Printf("[%d] Adding to hot layer...", id)
	ci, err := c.FFS.AddToHot(ctx, ior)
	if err != nil {
		return fmt.Errorf("importing data to hot storage (ipfs node): %s", err)
	}

	log.Infof("[%d] Pushing %s to FFS...", id, *ci)
	fmt.Printf("[%d] Pushing %s to FFS...", id, *ci)

	// For completeness, fields that could be relied on defaults
	// are explicitly kept here to have a better idea about their
	// existence.
	// This configuration will stop being static when we incorporate
	// other test cases.
	cidConfig := ffs.CidConfig{
		Cid:        *ci,
		Repairable: false,
		Hot: ffs.HotConfig{
			Enabled:       true,
			AllowUnfreeze: false,
			Ipfs: ffs.IpfsConfig{
				AddTimeout: 30,
			},
		},
		Cold: ffs.ColdConfig{
			Enabled: true,
			Filecoin: ffs.FilConfig{
				RepFactor:       1,
				DealMinDuration: 1000,
				Addr:            addr,
				CountryCodes:    nil,
				ExcludedMiners:  nil,
				TrustedMiners:   []string{minerAddr},
				Renew:           ffs.FilRenew{},
			},
		},
	}

	jid, err := c.FFS.PushConfig(ctx, *ci, client.WithCidConfig(cidConfig))
	if err != nil {
		return fmt.Errorf("pushing to FFS: %s", err)
	}

	log.Infof("[%d] Pushed successfully, queued job %s. Waiting for termination...", id, jid)
	fmt.Printf("[%d] Pushed successfully, queued job %s. Waiting for termination...", id, jid)
	chJob := make(chan client.JobEvent, 1)
	ctxWatch, cancel := context.WithCancel(ctx)
	defer cancel()
	err = c.FFS.WatchJobs(ctxWatch, chJob, jid)
	if err != nil {
		return fmt.Errorf("opening listening job status: %s", err)
	}
	var s client.JobEvent
	for s = range chJob {
		if s.Err != nil {
			return fmt.Errorf("job watching: %s", s.Err)
		}
		log.Infof("[%d] Job changed to status %s", id, ffs.JobStatusStr[s.Job.Status])
		if s.Job.Status == ffs.Failed || s.Job.Status == ffs.Canceled {
			return fmt.Errorf("job execution failed or was canceled")
		}
		if s.Job.Status == ffs.Success {
			return nil
		}
	}
	return fmt.Errorf("unexpected Job status watcher")
}

func main() {
	path, err := os.Getwd()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(path)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("hello")
		/*
			ma, err := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/5002")
			if err != nil {
				log.WithError(err).Error("error parsing multiaddress")
				// return "", "", err
			}
			client, err := client.NewClient(ma, grpc.WithInsecure())
			if err != nil {
				log.WithError(err).Error("failed to create powergate client")
				// return "", "", err
			}

			ffsID, ffsToken, err := client.FFS.Create(context.Background())
			fmt.Println("ffsID", ffsID, "ffsToken", ffsToken)
			if err != nil {
				log.WithError(err).Error("failed to create powergate ffs instance")
				// return "", "", err
			}

			err = client.Close()
			if err != nil {
				log.WithError(err).Error("failed to close powergate client")
			}
		*/

		setup := Setup{
			LotusAddr:    lotusAddr,
			MinerAddr:    "t01000",
			SampleSize:   700,
			MaxParallel:  1,
			TotalSamples: 1,
			RandSeed:     22,
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		err := Run(ctx, setup)
		if err != nil {
			fmt.Println(err)
		}

	})

	// Don't allow accessing /streams/ route
	// (only allow `/streams/:uuid/` or `/streams/:uuid/:uuid.m3u8`)
	http.HandleFunc("/streams/", func(w http.ResponseWriter, r *http.Request) {
		// serves the stream of original resolution
		if r.URL.Path != "/streams/" {
			println(path + " now " + r.URL.Path)
			http.ServeFile(w, r, r.URL.Path[1:])
		} else {
			println("no id provided!")
			http.NotFound(w, r)
			return
		}
	})

	http.HandleFunc("/streamsabr/", func(w http.ResponseWriter, r *http.Request) {
		// serves the stream of specified resolution
		if r.URL.Path != "/streamsabr/" {
			println(path + " now " + r.URL.Path)
			http.ServeFile(w, r, r.URL.Path[1:])
		} else {
			println("no id provided!")
			http.NotFound(w, r)
			return
		}
	})

	http.HandleFunc("/submitabr/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/submitabr/" {
			http.NotFound(w, r)
			return
		}

		if r.Method == "GET" {
			fmt.Fprintf(w, "GET, %q", html.EscapeString(r.URL.Path))
		} else if r.Method == "POST" {
			println(r.FormValue("input"))
			id := guuid.New()
			cmd1 := exec.Command("mkdir", "streamsabr/"+id.String())
			cmd1.Output()
			outputPath := "./streamsabr/" + id.String()
			urlinput := r.FormValue("input")
			println("outpath", outputPath)

			cmda := exec.Command("ffmpeg", "-i", urlinput, "-profile:v", "baseline", "-level", "3.0", "-s", "640x360", "-start_number", "0", "-hls_time", "10", "-hls_list_size", "0", "-f", "hls", outputPath+"/360p.m3u8")
			cmda.Output()
			cmdb := exec.Command("ffmpeg", "-i", urlinput, "-profile:v", "baseline", "-level", "3.0", "-s", "842x480", "-start_number", "0", "-hls_time", "10", "-hls_list_size", "0", "-f", "hls", outputPath+"/480p.m3u8")
			cmdb.Output()
			cmdc := exec.Command("ffmpeg", "-i", urlinput, "-profile:v", "baseline", "-level", "3.0", "-s", "1280x720", "-start_number", "0", "-hls_time", "10", "-hls_list_size", "0", "-f", "hls", outputPath+"/720p.m3u8")
			cmdc.Output()
			cmdd := exec.Command("ffmpeg", "-i", urlinput, "-profile:v", "baseline", "-level", "3.0", "-s", "1920x1080", "-start_number", "0", "-hls_time", "10", "-hls_list_size", "0", "-f", "hls", outputPath+"/1080p.m3u8")
			cmdd.Output()

			if err != nil {
				println(err.Error())
				if err.Error() == "exit status 1" {
					fmt.Fprintf(w, "Please use a valid url (which returns a mp4 file)")
					println("Please use a valid url (which returns a mp4 file)")
				}
				return
			}
			hostURL := "http://" + r.Host + "/streamsabr/"
			streamResponseABR := StreamResponseABR{
				id.String(),
				hostURL + id.String() + "/360p.m3u8",
				hostURL + id.String() + "/480p.m3u8",
				hostURL + id.String() + "/720p.m3u8",
				hostURL + id.String() + "/1080p.m3u8",
			}
			js, err2 := json.Marshal(streamResponseABR)
			if err2 != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.Write(js)

		} else {
			http.Error(w, "Invalid request method.", 405)
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
