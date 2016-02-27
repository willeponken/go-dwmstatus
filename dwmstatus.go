package main

// #cgo LDFLAGS: -lX11 -lasound
// #include <X11/Xlib.h>
// #include "getvol.h"
import "C"

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

type energyPaths struct {
	now  string
	full string
}

var (
	dpy                = C.XOpenDisplay(nil)
	currentEnergyPaths energyPaths
	foundEnergyPaths   = false
	batteryWarning     = false
)

func getVolumePerc() int {
	return int(C.get_volume_perc())
}

func pathExist(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}

	return true
}

func checkEnergyPaths(path string) {
	// pathList contains different file names the energy status
	// can be located in
	pathList := []energyPaths{
		energyPaths{
			now:  "BAT0/energy_now",
			full: "BAT0/energy_full",
		},
		energyPaths{
			now:  "BAT0/charge_now",
			full: "BAT0/charge_full",
		},
		energyPaths{
			now:  "BAT1/energy_now",
			full: "BAT1/energy_full",
		},
		energyPaths{
			now:  "BAT1/charge_now",
			full: "BAT1/charge_full",
		},
	}

	for _, paths := range pathList {
		if pathExist(fmt.Sprintf("%s/%s", path, paths.now)) && pathExist(fmt.Sprintf("%s/%s", path, paths.full)) {
			currentEnergyPaths = paths
			foundEnergyPaths = true
			return
		}
	}
}

func parseBatteryData(path string) (energy int) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		energy = -1
		return
	}

	fmt.Sscanf(string(data), "%d", &energy)

	return
}

func getBatteryStatus(path string) (status string, err error) {
	// Check if the energy paths are already found, if so, skip checking them again
	if foundEnergyPaths == false {
		checkEnergyPaths(path)
	}

	energyNow := parseBatteryData(fmt.Sprintf("%s/%s", path, currentEnergyPaths.now))
	energyFull := parseBatteryData(fmt.Sprintf("%s/%s", path, currentEnergyPaths.full))

	if energyNow == -1 || energyFull == -1 {
		status = "Unable to read battery data"
		err = errors.New(status)
		return
	}

	perc := energyNow * 100 / energyFull
	var warning string

	if perc < 15 && batteryWarning == false {
		batteryWarning = true
		warning = " [low]"
	} else {
		batteryWarning = false
	}

	status = fmt.Sprintf("%d%%%s", energyNow*100/energyFull, warning)
	return
}

func getLoadAverage(file string) (lavg string, err error) {
	loadavg, err := ioutil.ReadFile(file)
	if err != nil {
		return "Couldn't read loadavg", err
	}
	lavg = strings.Join(strings.Fields(string(loadavg))[:3], " ")
	return
}

func setStatus(s *C.char) {
	C.XStoreName(dpy, C.XDefaultRootWindow(dpy), s)
	C.XSync(dpy, 1)
}

func nowPlaying(addr string) (np string, err error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		np = "Couldn't connect to mpd."
		return
	}
	defer conn.Close()
	reply := make([]byte, 512)
	conn.Read(reply) // The mpd OK has to be read before we can actually do things.

	message := "status\n"
	conn.Write([]byte(message))
	conn.Read(reply)
	r := string(reply)
	arr := strings.Split(string(r), "\n")
	if arr[8] != "state: play" { //arr[8] is the state according to the mpd documentation
		status := strings.SplitN(arr[8], ": ", 2)
		np = fmt.Sprintf("mpd - [%s]", status[1]) //status[1] should now be stopped or paused
		return
	}

	message = "currentsong\n"
	conn.Write([]byte(message))
	conn.Read(reply)
	r = string(reply)
	arr = strings.Split(string(r), "\n")
	if len(arr) > 5 {
		var artist, title string
		for _, info := range arr {
			field := strings.SplitN(info, ":", 2)
			switch field[0] {
			case "Artist":
				artist = strings.TrimSpace(field[1])
			case "Title":
				title = strings.TrimSpace(field[1])
			default:
				//do nothing with the field
			}
		}
		np = artist + " - " + title
		return
	}
	//This is a nonfatal error.
	np = "Playlist is empty."
	return
}

func formatStatus(format string, args ...interface{}) *C.char {
	status := fmt.Sprintf(format, args...)
	return C.CString(status)
}

func main() {
	if dpy == nil {
		log.Fatal("Can't open display")
	}

	for {
		t := time.Now().Format("Mon 02 15:04")

		b, err := getBatteryStatus("/sys/class/power_supply")
		if err != nil {
			log.Println(err)
		}

		l, err := getLoadAverage("/proc/loadavg")
		if err != nil {
			log.Println(err)
		}

		m, err := nowPlaying("localhost:6600")
		if err != nil {
			log.Println(err)
		}

		vol := getVolumePerc()

		// TODO Add flags to disable certain status parts (mpd, loadavg etc.)
		s := formatStatus("%s :: %d%% :: %s :: %s :: %s", m, vol, l, t, b)
		setStatus(s)
		time.Sleep(time.Second)
	}
}
