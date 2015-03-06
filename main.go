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
	"net/http"
	"os"
	"path"
	"path/filepath"
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
	*root_name, _ = filepath.Abs(*root_name)

	// simple use-case: scan one directory and dump the created
	// packages-list to stdout.
	if *dump_package_list {
		now := time.Now()
		log.Println("start building index from", *root_name)

		packages, err := ScanDirectoryForPackages(*root_name, *nworkers, *add_md5, *add_sha1)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(2)
		}
		log.Println("done building index")
		log.Printf("time to parse %d packages: %s\n", len(packages.Entries), time.Since(now))

		os.Stdout.WriteString(packages.String())
		return
	}

	// regular use-case: serve the given directory + the Packages file(s)
	// recursively.

	gzipper := Gzipper(GzGzipPipe)
	if !*use_gzip {
		gzipper = GzGolang
	}

	indices := make([]string, 0)

	filepath.Walk(*root_name, func(path string, fi os.FileInfo, err error) error {

		if !fi.IsDir() {
			return nil
		}

		var (
			packages *PackageIndex
			now      = time.Now()
		)

		log.Printf("start building index for %q", path)

		if packages, err = ScanDirectoryForPackages(path, *nworkers, *add_md5, *add_sha1); err != nil {
			log.Printf("error: %v", err)
			return nil
		}

		log.Printf("done building index for %q", path)
		log.Printf("time to parse %d packages in %q: %s\n", len(packages.Entries), path, time.Since(now))

		mux_path := path[len(*root_name):]
		if mux_path == "" {
			mux_path = "/"
		}

		// non-package directories
		if len(packages.Entries) == 0 {
			http.Handle(mux_path, http.FileServer(http.Dir(path)))
			return nil
		}

		AttachHttpHandler(http.DefaultServeMux, packages, mux_path, *root_name, gzipper)

		indices = append(indices, mux_path)

		return nil
	})
	AttachOpkgRepoSnippet(http.DefaultServeMux, "/opkg.conf", indices)

	log.Println()
	log.Printf("serving at %s", "http://"+listen.Addr().String())
	http.Serve(listen, nil)
}

func ScanDirectoryForPackages(dir string, nworkers int, add_md5, add_sha1 bool) (*PackageIndex, error) {

	root, err := os.Open(dir)
	if err != nil {
		return nil, fmt.Errorf("opening -root %q: %v\n", dir, err)
	}

	entries, err := root.Readdirnames(-1)
	if err != nil {
		return nil, fmt.Errorf("reading dir entries from -root %q: %v\n", dir, err)
	}

	packages := &PackageIndex{Entries: make(map[string]*Ipkg)}
	workers := NewWorkerPool(nworkers)

	for _, entry := range entries {
		if path.Ext(entry) != ".ipk" {
			continue
		}
		workers.Hire()
		go func(name string) {
			defer workers.Release()
			ipkg, err := NewIpkgFromFile(name, dir, add_md5, add_sha1)
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
	return packages, nil
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
