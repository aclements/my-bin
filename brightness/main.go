package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/sixdouglas/suncalc"
)

const updatePeriod = 5 * time.Minute
const fadeDuration = time.Hour
const latitude = 42.3601
const longitude = -71.0589

func main() {
	var lastBrightness float64 = -1
	plugAndPlay, err := monitorMonitor()
	if err != nil {
		log.Fatal(err)
	}
	for {
		b := getBrightness(time.Now())
		if b != lastBrightness {
			lastBrightness = b
			scaled := b*(0.5-0.1) + 0.1
			setBrightness(scaled)
		}
		select {
		case <-plugAndPlay:
		case <-time.After(updatePeriod):
		}
	}
}

func monitorMonitor() (<-chan struct{}, error) {
	udevadm := exec.Command("udevadm", "monitor", "-u", "--subsystem-match=drm")
	udevadm.Stderr = os.Stderr
	out, err := udevadm.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := udevadm.Start(); err != nil {
		return nil, err
	}

	// Read the udevadm output. This will look like:
	//
	// $ udevadm monitor -u --subsystem-match=drm
	// monitor will print the received events for:
	// UDEV - the event which udev sends out after rule processing
	//
	// UDEV  [289027.964180] change   /devices/pci0000:00/0000:00:02.0/drm/card0 (drm)
	//
	// Rather than try to interpret all of this, we just take any output as a
	// signal that we should check te screen, and debounce the output.

	ch := make(chan struct{})
	go func() {
		var buf [128]byte
		for {
			_, err := out.Read(buf[:])
			if err == io.EOF {
				break
			} else if err != nil {
				log.Fatal("error reading from udevadm: ", err)
			}
			ch <- struct{}{}
		}

		udevadm.Process.Kill()
	}()
	go func() {
		// We don't expect udevadm to ever exit.
		if err := udevadm.Wait(); err != nil {
			log.Printf("udevadm exited unexpectedly: %s", err)
		}
	}()

	return debounce(ch, 100*time.Millisecond), nil
}

func debounce[T any](ch <-chan T, delay time.Duration) <-chan T {
	out := make(chan T)
	go func() {
		for val := range ch {
			// Consume anything extra from ch until delay.
			ready := time.After(delay)
		consume:
			for {
				select {
				case _, ok := <-ch:
					if !ok {
						break consume
					}
				case <-ready:
					break consume
				}
			}
			out <- val
		}
		close(out)
	}()
	return out
}

func listToday() {
	y, m, d := time.Now().Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, time.Local)
	for dt := time.Duration(0); dt < 24*time.Hour; dt += 20 * time.Minute {
		t := start.Add(dt)
		fmt.Printf("%s %g\n", t, getBrightness(t))
	}
}

func getBrightness(now time.Time) float64 {
	times := suncalc.GetTimes(now, latitude, longitude)

	if now.Before(times[suncalc.Sunrise].Value) {
		return 0
	}

	full := times[suncalc.Sunrise].Value.Add(fadeDuration)
	if now.Before(full) {
		return 1 - float64(full.Sub(now))/float64(fadeDuration)
	}

	dim := times[suncalc.Sunset].Value.Add(-fadeDuration)
	if now.Before(dim) {
		return 1
	}

	if now.Before(times[suncalc.Sunset].Value) {
		return float64(now.Sub(dim)) / float64(fadeDuration)
	}

	return 0
}

func setBrightness(val float64) {
	// My home monitor is 112NTYTJB258. The VCP for brightness is 0x10 and its
	// value is between 0 and 100.
	v := fmt.Sprint(int(val * 100))
	cmd := exec.Command("ddcutil", "setvcp", "--sn", "112NTYTJB258", "0x10", v)
	out, err := cmd.CombinedOutput()
	if err == nil || string(out) == "Display not found\n" {
		return
	}
	log.Fatalf("%s failed: %s\n%s", cmd, err, out)
}
