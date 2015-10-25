package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/mux"
)

func main() {
	log("blocker: starting up...\n")

	d, err := NewEbsVolumeDriver()
	if err != nil {
		logError("Failed to create an EBS driver: %s.\n", err)
		return
	}

	// Done is a channel that signals program exit.
	done := make(chan bool, 1)

	// Listen to important OS signals, so we trigger exit cleanly.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, os.Kill, syscall.SIGTERM)
	go func() {
		sig := <-signals
		log("Caught signal %s: shutting down.\n", sig)
		// TODO: forcibly unmount all volumes.
		done <- true
	}()

	// Now listen for HTTP calls from Docker.
	go listen(d, true, done)

	// Block until the program exits.
	<-done
}

func listen(d VolumeDriver, socket bool, done chan bool) {
	handler := makeRoutes(d)

	if socket {
		const SocketFile = "/var/run/blocker/blocker.sock"

		l, err := net.Listen("unix", SocketFile)
		if err != nil {
			logError("Failed to listen on socket %s: %s.\n", SocketFile, err)
		} else {
			defer l.Close()

			log("Ready to go; listening on socket %s...\n", SocketFile)
			err = http.Serve(l, handler)
			if err != nil {
				logError("HTTP server error: %s.\n", err)
			}
		}
	} else {
		const ListenAddress = ":1234"

		log("Ready to go; listening on port %s...\n", ListenAddress)
		err := http.ListenAndServe(ListenAddress, handler)
		if err != nil {
			logError("HTTP server error: %s.\n", err)
		}
	}

	done <- true
}

func makeRoutes(d VolumeDriver) http.Handler {
	r := mux.NewRouter()
	// TODO: permit options in the name string.
	r.HandleFunc("/Plugin.Activate", servePluginActivate)
	r.HandleFunc("/VolumeDriver.Create", serveVolumeSimple(d.Create))
	r.HandleFunc("/VolumeDriver.Mount", serveVolumeComplex(d.Mount))
	r.HandleFunc("/VolumeDriver.Path", serveVolumeComplex(d.Path))
	r.HandleFunc("/VolumeDriver.Remove", serveVolumeSimple(d.Remove))
	r.HandleFunc("/VolumeDriver.Unmount", serveVolumeSimple(d.Unmount))
	return r
}

type pluginInfoResponse struct {
	Implements []string
}

func servePluginActivate(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(pluginInfoResponse{
		Implements: []string{"VolumeDriver"},
	})
}

type volumeRequest struct {
	Name string
}

type volumeSimpleResponse struct {
	Err string
}

func serveVolumeSimple(f func(string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log("* %s\n", r.URL.String())
		var vol volumeRequest
		err := json.NewDecoder(r.Body).Decode(&vol)
		if err == nil {
			err = f(vol.Name)
			log("\tdone: (%s): %v\n", vol.Name, err)
		}
		var errs string
		if err != nil {
			errs = err.Error()
		}
		json.NewEncoder(w).Encode(volumeSimpleResponse{
			Err: errs,
		})
	}
}

type volumeComplexResponse struct {
	Mountpoint string
	Err        string
}

func serveVolumeComplex(f func(string) (string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log("* %s\n", r.URL.String())
		var vol volumeRequest
		err := json.NewDecoder(r.Body).Decode(&vol)
		var mountpoint string
		if err == nil {
			mountpoint, err = f(vol.Name)
			log("\tdone: (%s): (%s, %v)\n", vol.Name, mountpoint, err)
		}
		var errs string
		if err != nil {
			errs = err.Error()
		}
		json.NewEncoder(w).Encode(volumeComplexResponse{
			Mountpoint: mountpoint,
			Err:        errs,
		})
	}
}
