// This file is part of *kellner*
//
// Copyright (C) 2015, Travelping GmbH <copyright@travelping.com>
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"io"
	"log"
	"os"
)

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
