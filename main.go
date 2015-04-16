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
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const versionString = "kellner-0.3"

func main() {

	var (
		nworkers        = flag.Int("workers", 4, "number of workers")
		bind            = flag.String("bind", ":8080", "address to bind to")
		rootName        = flag.String("root", "", "directory containing the packages")
		dumpPackageList = flag.Bool("dump", false, "just dump the package list and exit")
		addMd5          = flag.Bool("md5", true, "calculate md5 of scanned packages")
		addSha1         = flag.Bool("sha1", false, "calculate sha1 of scanned packages")
		useGzip         = flag.Bool("gzip", true, "use 'gzip' to compress the package index. if false: use golang")
		showVersion     = flag.Bool("version", false, "show version and exit")
		logFileName     = flag.String("log", "", "log to given filename")

		tlsKey               = flag.String("tls-key", "", "PEM encoded ssl-key")
		tlsCert              = flag.String("tls-cert", "", "PEM encoded ssl-cert")
		tlsClientCas         = flag.String("tls-client-ca-file", "", "file with PEM encoded list of ssl-certs containing the CAs")
		tlsRequireClientCert = flag.Bool("require-client-cert", false, "require a client-cert")
		tlsClientIDMuxRoot   = flag.String("idmap", "", "directory containing the client-mappings")
		printClientCert      = flag.String("print-client-cert-id", "", "print client-id for given .cert and exit")

		listen net.Listener
		err    error
	)

	flag.Parse()

	if *showVersion {
		fmt.Println(versionString)
		return
	}

	if *printClientCert != "" {
		if err = printClientIDTo(os.Stdout, *printClientCert); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *bind == "" {
		fmt.Fprintf(os.Stderr, "usage error: missing / empty -bind\n")
		os.Exit(1)
	}

	if *rootName == "" {
		fmt.Fprintf(os.Stderr, "usage error: missing / empty -root")
		os.Exit(1)
	}
	*rootName, _ = filepath.Abs(*rootName)

	var logger io.Writer = os.Stderr
	var logFile *os.File
	if *logFileName != "" {
		logFile, err = os.OpenFile(*logFileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "can't create -log %q: %v", *logFileName, err)
			os.Exit(1)
		}
		logger = io.MultiWriter(os.Stderr, logFile)
	}
	log.SetOutput(logger)

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

	// simple use-case: scan one directory and dump the created
	// packages-list to stdout.
	if *dumpPackageList {
		now := time.Now()
		log.Println("start building index from", *rootName)

		packages, err := scanDirectoryForPackages(*rootName, *nworkers, *addMd5, *addSha1)
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
	//
	// setup the listener: either ssl or pure tcp
	l, err := net.Listen("tcp", *bind)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: binding to %q failed: %v\n", *bind, err)
		os.Exit(1)
	}
	listen = l

	if *tlsCert != "" || *tlsKey != "" {

		var tlsOpts = tlsOptions{
			keyFileName:       *tlsKey,
			certFileName:      *tlsCert,
			requireClientCert: *tlsRequireClientCert,
			clientCasFileName: *tlsClientCas,
		}

		if listen, err = initTLS(listen, &tlsOpts); err != nil {

			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(2)
		}
	}

	log.Println("listen on", listen.Addr())

	gzwrite := gzWrite(gzGzipPipe)
	if !*useGzip {
		gzwrite = gzGolang
	}

	// the root-muxer is used either directly (non-ssl-client-cert case) or
	// as a lookup-pool for ClientIdMuxer to get the real worker
	rootMuxer := http.NewServeMux()

	startTime := time.Now()
	var indices = make([]string, 0)
	filepath.Walk(*rootName, func(path string, fi os.FileInfo, err error) error {

		if !fi.IsDir() {
			return nil
		}

		var (
			packages *packageIndex
			now      = time.Now()
		)

		log.Printf("start building index for %q", path)

		if packages, err = scanDirectoryForPackages(path, *nworkers, *addMd5, *addSha1); err != nil {
			log.Printf("error: %v", err)
			return nil
		}

		log.Printf("done building index for %q", path)
		log.Printf("time to parse %d packages in %q: %s\n", len(packages.Entries), path, time.Since(now))

		muxPath := path[len(*rootName):]
		if muxPath == "" {
			muxPath = "/"
		}

		// non-package directories
		if len(packages.Entries) == 0 {
			rootMuxer.Handle(muxPath, http.FileServer(http.Dir(path)))
			return nil
		}

		attachHTTPHandler(rootMuxer, packages, muxPath, *rootName, gzwrite)

		indices = append(indices, muxPath)

		return nil
	})
	// TODO: this is specific to non-client-id situations
	attachOpkgRepoSnippet(rootMuxer, "/opkg.conf", indices)

	log.Println()
	log.Printf("processed %d package-folders in %s", len(indices), time.Since(startTime))

	var httpHandler http.Handler = rootMuxer
	if *tlsClientIDMuxRoot != "" {
		httpHandler = &clientIDMuxer{
			Folder: *tlsClientIDMuxRoot,
			Muxer:  rootMuxer,
		}
	}

	httpHandler = logRequests(httpHandler)

	log.Println()
	proto := "http://"
	if *tlsKey != "" {
		proto = "https://"
	}
	log.Printf("serving at %s", proto+listen.Addr().String())
	http.Serve(listen, httpHandler)
}

func scanDirectoryForPackages(dir string, nworkers int, addMd5, addSha1 bool) (*packageIndex, error) {

	root, err := os.Open(dir)
	if err != nil {
		return nil, fmt.Errorf("opening -root %q: %v\n", dir, err)
	}

	entries, err := root.Readdirnames(-1)
	if err != nil {
		return nil, fmt.Errorf("reading dir entries from -root %q: %v\n", dir, err)
	}

	packages := &packageIndex{Entries: make(map[string]*ipkArchive)}
	workers := newWorkerPool(nworkers)

	for _, entry := range entries {
		if path.Ext(entry) != ".ipk" {
			continue
		}
		workers.Hire()
		go func(name string) {
			defer workers.Release()
			archive, err := newIpkFromFile(name, dir, addMd5, addSha1)
			if err != nil {
				log.Printf("error: %v\n", err)
				return
			}
			packages.Lock()
			packages.Entries[name] = archive
			packages.Unlock()
		}(entry)
	}
	workers.Wait()
	return packages, nil
}

type workerPool struct {
	sync.WaitGroup
	worker chan bool
}

func newWorkerPool(n int) *workerPool {
	return &workerPool{worker: make(chan bool, n)}
}

// hire / block a worker from the pool
func (pool *workerPool) Hire() {
	pool.worker <- true
	pool.Add(1)
}

// release / unblock a blocked worker from the pool
func (pool *workerPool) Release() {
	pool.Done()
	<-pool.worker
}
