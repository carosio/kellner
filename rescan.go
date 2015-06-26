package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// on SIGUSR2 kellner rescans root. if another scan is already running
// we just ignore the signal.
func rescan(root, cache string, nworkers int, doMD5, doSHA1 bool, gzipper gzWrite) {

	var (
		sigChan = make(chan os.Signal, 1)
		doScan  = make(chan bool, 1)
	)

	doScan <- true
	signal.Notify(sigChan, syscall.SIGUSR2)

	for sig := range sigChan {
		switch sig {
		case syscall.SIGUSR2:
			log.Println("info: received SIGUSR2")

			// make sure only one scan-process is running at any
			// given time:
			select {
			case <-doScan:
				go func() {
					log.Println("info: start a rescan.")
					scanRoot(root, cache, nworkers, doMD5, doSHA1, gzipper)
					doScan <- true
				}()
			case <-time.After(1 * time.Millisecond):
				log.Println("info: another rescan is currently processed, please retry when the other scan is done.")
				continue
			}
		}
	}
}
