// This file is part of *kellner*
//
// Copyright (C) 2015, Travelping GmbH <copyright@travelping.com>
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

// *kellner* scans package files in a given directory
// and builds a Packages.gz file on the fly. it then serves the
// Packages.gz and the .ipk files by the built-in httpd
// and is ready to be used from opkg
//
// related tools:
// * https://github.com/17twenty/opkg-scanpackages
// * opkg-make-index from the opkg-utils collection

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path"
	"sync"
	"time"
)

const VERSION = "kellner-0.1"

func main() {

	var (
		nworkers          = flag.Int("workers", 4, "number of workers")
		bind              = flag.String("bind", ":8080", "address to bind to")
		root_name         = flag.String("root", "", "directory containing the packages")
		dump_package_list = flag.Bool("dump", false, "just dump the package list and exit")
		add_md5           = flag.Bool("md5", true, "calculate md5 of scanned packages")
		add_sha1          = flag.Bool("sha1", true, "calculate sha1 of scanned packages")
		use_gzip          = flag.Bool("gzip", true, "use 'gzip' to compress the package index. if false: use golang")
		show_version      = flag.Bool("version", false, "show version")
		listen            net.Listener
	)

	flag.Parse()

	if *show_version {
		fmt.Println(VERSION)
		return
	}

	if *bind == "" {
		fmt.Fprintf(os.Stderr, "usage error: missing / empty -bind\n")
		os.Exit(1)
	}
	if !*dump_package_list {
		l, err := net.Listen("tcp", *bind)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: binding to %q failed: %v\n", *bind, err)
			os.Exit(1)
		}
		listen = l
		log.Println("listen to", l.Addr())
	}

	if *root_name == "" {
		fmt.Fprintf(os.Stderr, "usage error: missing / empty -root")
	}

	root, err := os.Open(*root_name)
	if err != nil {
		fmt.Printf("error: opening -root %q: %v\n", *root_name, err)
		os.Exit(1)
	}

	log.Println("start building index from", *root_name)
	now := time.Now()
	entries, err := root.Readdirnames(-1)
	if err != nil {
		fmt.Printf("error: reading dir entries from -root %q: %v\n", *root_name, err)
		os.Exit(1)
	}

	//
	// create package list
	//
	packages := PackageIndex{Entries: make(map[string]*Ipkg)}
	workers := NewWorkerPool(*nworkers)

	for _, entry := range entries {
		if path.Ext(entry) != ".ipk" {
			continue
		}
		workers.Hire()
		go func(name string) {
			defer workers.Release()
			ipkg, err := NewIpkgFromFile(name, *root_name, *add_md5, *add_sha1)
			if err != nil {
				log.Printf("error: %v\n", err)
				return
			}
			packages.Lock()
			packages.Entries[name] = ipkg
			packages.Unlock()
		}(entry)
	}
	workers.Wait()

	log.Println("done building index")
	log.Printf("time to parse %d packages: %s\n", len(packages.Entries), time.Since(now))

	if *dump_package_list {
		os.Stdout.WriteString(packages.String())
		return
	}

	gzipper := Gzipper(GzGzipPipe)
	if !*use_gzip {
		gzipper = GzGolang
	}

	ServeHTTP(&packages, *root_name, gzipper, listen)
}

type WorkerPool struct {
	sync.WaitGroup
	worker chan bool
}

func NewWorkerPool(n int) *WorkerPool {
	return &WorkerPool{worker: make(chan bool, n)}
}

// hire / block a worker from the pool
func (pool *WorkerPool) Hire() {
	pool.worker <- true
	pool.Add(1)
}

// release / unblock a blocked worker from the pool
func (pool *WorkerPool) Release() {
	pool.Done()
	<-pool.worker
}
