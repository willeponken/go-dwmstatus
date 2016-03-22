package main

// #cgo LDFLAGS: -lX11 -lasound
// #include <X11/Xlib.h>
// #include "getvol.h"
import "C"

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"strings"
	"time"
)

type energyPaths struct {
	now  string
	full string
}

type flags struct {
	battery     bool
	time        bool
	loadavg     bool
	mpd         bool
	volume      bool
	batteryNow  string
	batteryFull string
	timeFormat  string
}

var context = flags{
	battery:     true,
	time:        true,
	loadavg:     true,
	mpd:         true,
	volume:      true,
	batteryNow:  "/sys/class/power_supply/BAT0/energy_now",
	batteryFull: "/sys/class/power_supply/BAT0/energy_full",
	timeFormat:  "Mon 02 15:04",
}

var (
	dpy                = C.XOpenDisplay(nil)
	currentEnergyPaths energyPaths
	foundEnergyPaths   = false
	batteryWarning     = false
)

func getVolumePerc() string {
	return fmt.Sprintf("%d%%", int(C.get_volume_perc()))
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

func getBatteryStatus(pathNow string, pathFull string) (status string, err error) {
	energyNow := parseBatteryData(pathNow)
	energyFull := parseBatteryData(pathFull)

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

	var artist, title string

	r = string(reply)
	arr = strings.Split(string(r), "\n")
	if len(arr) > 5 {
		for _, info := range arr {
			field := strings.SplitN(info, ":", 2)
			switch field[0] {
			case "Artist":
				artist = strings.TrimSpace(field[1])
			case "Title":
				title = strings.TrimSpace(field[1])
			case "Name":
				if artist == "" || title == "" {
					np = strings.TrimSpace(field[1]) // no artist nor title, set name of stream.
				}
			}
		}

		if np == "" {
			if artist != "" && title != "" {
				np = artist + " - " + title
			} else if title != "" {
				np = title
			} else {
				np = artist
			}
		}

		return
	}

	//This is a nonfatal error.
	np = "Playlist is empty."
	return
}

func init() {
	flag.StringVar(&context.batteryNow, "battery-now", context.batteryNow, "path to current energy file")
	flag.StringVar(&context.batteryFull, "battery-full", context.batteryFull, "path to full energy file")
	flag.StringVar(&context.timeFormat, "time-format", context.timeFormat, "time format")

	flag.BoolVar(&context.battery, "battery", context.battery, "show battery indicator")
	flag.BoolVar(&context.time, "time", context.time, "show time indictator")
	flag.BoolVar(&context.loadavg, "loadavg", context.loadavg, "show load average")
	flag.BoolVar(&context.mpd, "mpd", context.mpd, "show mpd status")
	flag.BoolVar(&context.volume, "volume", context.volume, "show current volume")

	flag.Parse()
}

func main() {
	if dpy == nil {
		log.Fatal("Can't open display")
	}

	var indicators [5]string
	var err error

	for {
		index := 0

		if context.mpd {
			indicators[index], err = nowPlaying("localhost:6600")
			index++

			if err != nil {
				log.Println(err)
			}
		}

		if context.volume {
			indicators[index] = getVolumePerc()
			index++
		}

		if context.loadavg {
			indicators[index], err = getLoadAverage("/proc/loadavg")
			index++

			if err != nil {
				log.Println(err)
			}
		}

		if context.time {
			indicators[index] = time.Now().Format(context.timeFormat)
			index++
		}

		if context.battery {
			indicators[index], err = getBatteryStatus(context.batteryNow, context.batteryFull)
			index++

			if err != nil {
				log.Println(err)
			}
		}

		var status string
		for i := 0; i < index; i++ {
			status += indicators[i]

			if i < index-1 {
				status += " :: "
			}
		}

		setStatus(C.CString(status))

		time.Sleep(time.Second)
	}
}
