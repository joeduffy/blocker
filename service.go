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

const SocketFile = "/var/run/blocker.sock"

func main() {
	log("blocker: starting up...\n")

	d, err := NewEbsVolumeDriver()
	if err != nil {
		logError("Failed to create an EBS driver: %s.\n", err)
		return
	}

	// Manufacture a socket for communication with Docker.
	l, err := net.Listen("unix", SocketFile)
	if err != nil {
		logError("Failed to listen on socket %s: %s.\n", SocketFile, err)
		return
	}
	defer l.Close()

	// Make a channel that signals program exit.
	exit := make(chan bool, 1)

	// Listen to important OS signals, so we trigger exit cleanly.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, os.Kill, syscall.SIGTERM)
	go func() {
		sig := <-signals
		log("Caught signal %s: shutting down.\n", sig)
		// TODO: forcibly unmount all volumes.
		exit <- true
	}()

	// Now listen for HTTP calls from Docker.
	handler := makeRoutes(d)
	go func() {
		log("Ready to go; listening on socket %s...\n", SocketFile)
		err = http.Serve(l, handler)
		if err != nil {
			logError("HTTP server error: %s.\n", err)
		}
		exit <- true
	}()

	// Block until the program exits.
	<-exit
}

func makeRoutes(d VolumeDriver) http.Handler {
	r := mux.NewRouter()
	r.HandleFunc("/Plugin.Activate", servePluginActivate)
	r.HandleFunc("/VolumeDriver.Create", serveVolumeDriverCreate(d))
	r.HandleFunc("/VolumeDriver.Mount", serveVolumeDriverMount(d))
	r.HandleFunc("/VolumeDriver.Path", serveVolumeDriverPath(d))
	r.HandleFunc("/VolumeDriver.Remove", serveVolumeDriverRemove(d))
	r.HandleFunc("/VolumeDriver.Unmount", serveVolumeDriverUnmount(d))
	return r
}

// Plugin.Activate:

type pluginActivateResponse struct {
	Implements []string
}

func servePluginActivate(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(pluginActivateResponse{
		Implements: []string{"VolumeDriver"},
	})
}

// VolumeDriver.Create:

type volumeDriverCreateRequest struct {
	Name string
	Opts map[string]string
}

type volumeDriverCreateResponse struct {
	Err string
}

func serveVolumeDriverCreate(d VolumeDriver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log("* %s\n", r.URL.String())
		var req volumeDriverCreateRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		if err == nil {
			err = d.Create(req.Name, req.Opts)
			log("\tdone: (%s): %v\n", req.Name, err)
		}
		var errs string
		if err != nil {
			errs = err.Error()
		}
		json.NewEncoder(w).Encode(volumeDriverCreateResponse{
			Err: errs,
		})
	}
}

// VolumeDriver.Mount:

type volumeDriverMountRequest struct {
	Name string
}

type volumeDriverMountResponse struct {
	Mountpoint string
	Err        string
}

func serveVolumeDriverMount(d VolumeDriver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log("* %s\n", r.URL.String())
		var req volumeDriverMountRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		var mountpoint string
		if err == nil {
			mountpoint, err = d.Mount(req.Name)
			log("\tdone: (%s): (%s, %v)\n", req.Name, mountpoint, err)
		}
		var errs string
		if err != nil {
			errs = err.Error()
		}
		json.NewEncoder(w).Encode(volumeDriverMountResponse{
			Mountpoint: mountpoint,
			Err:        errs,
		})
	}
}

// VolumeDriver.Path:

type volumeDriverPathRequest struct {
	Name string
}

type volumeDriverPathResponse struct {
	Mountpoint string
	Err        string
}

func serveVolumeDriverPath(d VolumeDriver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log("* %s\n", r.URL.String())
		var req volumeDriverPathRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		var mountpoint string
		if err == nil {
			mountpoint, err = d.Path(req.Name)
			log("\tdone: (%s): (%s, %v)\n", req.Name, mountpoint, err)
		}
		var errs string
		if err != nil {
			errs = err.Error()
		}
		json.NewEncoder(w).Encode(volumeDriverPathResponse{
			Mountpoint: mountpoint,
			Err:        errs,
		})
	}
}

// VolumeDriver.Remove:

type volumeDriverRemoveRequest struct {
	Name string
}

type volumeDriverRemoveResponse struct {
	Err string
}

func serveVolumeDriverRemove(d VolumeDriver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log("* %s\n", r.URL.String())
		var req volumeDriverRemoveRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		if err == nil {
			err = d.Remove(req.Name)
			log("\tdone: (%s): %v\n", req.Name, err)
		}
		var errs string
		if err != nil {
			errs = err.Error()
		}
		json.NewEncoder(w).Encode(volumeDriverRemoveResponse{
			Err: errs,
		})
	}
}

// VolumeDriver.Unmount:

type volumeDriverUnmountRequest struct {
	Name string
}

type volumeDriverUnmountResponse struct {
	Err string
}

func serveVolumeDriverUnmount(d VolumeDriver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log("* %s\n", r.URL.String())
		var req volumeDriverUnmountRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		if err == nil {
			err = d.Unmount(req.Name)
			log("\tdone: (%s): %v\n", req.Name, err)
		}
		var errs string
		if err != nil {
			errs = err.Error()
		}
		json.NewEncoder(w).Encode(volumeDriverUnmountResponse{
			Err: errs,
		})
	}
}
