// This file is part of *kellner*
//
// Copyright (C) 2015, Travelping GmbH <copyright@travelping.com>
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func setupLogging(logFileName string) {

	var (
		logger  io.Writer = os.Stderr
		logFile *os.File
		err     error
	)

	if logFileName != "" {
		logFile, err = os.OpenFile(logFileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "can't create -log %q: %v", logFileName, err)
			os.Exit(1)
		}
		logger = io.MultiWriter(os.Stderr, logFile)
	}
	log.SetOutput(logger)

	// on SIGUSR1 we rotate the log, see rotateLog()
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGUSR1) // NOTE: USR1 does not exist on windows
		for sig := range sigChan {
			switch sig {
			case syscall.SIGUSR1:

				log.Printf("received USR1, recreating log file")

				logFile, logger = rotateLog(logFile, logger)
				log.SetOutput(logger)
			}
		}
	}()
}

// assumption: user or logrotate has moved / renamed the file: we
// still have the handle to the file but the name is gone. so,
// we create a new file (and truncate! an existing one).
func rotateLog(currentLogFile *os.File, currentOutput io.Writer) (newLogFile *os.File, newOutput io.Writer) {

	if currentLogFile == nil {
		return currentLogFile, currentOutput
	}

	// first create the new file
	logFileName := currentLogFile.Name()
	logFile, err := os.Create(logFileName)
	if err != nil {
		log.Printf("error: can't create -log %q after USR1: %v", logFileName, err)
		return currentLogFile, currentOutput
	}

	// close the old file
	currentLogFile.Close()

	return logFile, io.MultiWriter(os.Stderr, logFile)
}
