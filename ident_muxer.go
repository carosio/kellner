// This file is part of *kellner*
//
// Copyright (C) 2015, Travelping GmbH <copyright@travelping.com>
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// the Identity-folder contains a bunch of subfolders. the name of each subfolder is
//
// root/
//      ipk-folder1/
//      ipk-folder2/
//      ipk-folder3/
//
// id-root/
//         client-id-1/
//                     ipk-folder1           (empty file, maps request "/ipk-folder1" to
//                                            /root/ipk-folder1 )
//                     special [ipk-folder2] (text file, containing "ipk-folder2",
//                                            maps request "/special" to /root/ipk-folder2 )
//
type ClientIdMuxer struct {
	IdRoot    string         // folder to use for lookup client-id-requests
	RootMuxer *http.ServeMux // hold the real worker
}

// looks up the first certificate to get the client-id. based upon the client-id
// we lookup the client-directory and, based upon the request, the mapping to the
// real handler.
func (muxer *ClientIdMuxer) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		writeError(http.StatusUnauthorized, w, r)
		return
	}

	cert := r.TLS.PeerCertificates[0]
	clientID := clientIdByName(&cert.Subject)

	// TODO: do we want this?
	w.Header().Set("Kellner-Client-Id", clientID)

	// TODO: decide how we treat "/" requests ...
	// we could deliver opkg.conf containing all the valid repos
	// or something like that?
	if r.URL.Path == "/" {
		writeError(http.StatusForbidden, w, r)
		return
	}

	var (
		requestedPath = path.Clean(r.URL.Path)
		mapFile       string
		fi            os.FileInfo
		err           error
	)

	// try to cut away components from the clientID and
	// and find a mapping file
	for clientID != "" {

		pos := strings.LastIndexAny(clientID, ",")
		if pos <= 0 {
			break
		}

		clientDir := filepath.Join(muxer.IdRoot, clientID)

		// first try  /cdir/request/file.ipk
		// than       /cdir/request
		//
		// both should find the mapping file /cdir/request
		// TODO:      /cidr/request/subfolder/file.ipk
		mapFile = filepath.Join(clientDir, requestedPath)
		if fi, err = os.Lstat(mapFile); err != nil {
			requestedPath = path.Dir(requestedPath)
			mapFile = filepath.Join(clientDir, requestedPath)
			fi, err = os.Lstat(mapFile)
		}

		if fi != nil {
			break
		}

		clientID = clientID[:pos]
	}

	if fi == nil {
		http.NotFound(w, r)
		return
	}

	// TODO: decide how to treat a directory
	if fi.IsDir() {
		log.Println("fi is a directory, is 404 ok?")
		http.NotFound(w, r)
		return
	}

	mappedPath := requestedPath

	// if the file is not empty, the content defines the mapping
	if fi.Size() > 0 {
		content, err := ioutil.ReadFile(mapFile)
		if err != nil {
			log.Printf("warning: reading %q yields %v", mapFile, err)
			writeError(http.StatusInternalServerError, w, r)
			return
		}
		mappedPath = string(bytes.TrimSpace(content))
	}

	mappedRequest := *r
	mappedRequest.URL, _ = url.Parse(r.URL.String())
	mappedRequest.URL.Path = cleanPath(path.Join(mappedPath, path.Base(r.URL.Path)))
	mappedRequest.RequestURI = mappedRequest.URL.Path

	handler, matchingPattern := muxer.RootMuxer.Handler(&mappedRequest)

	r.Header.Add(_EXTRA_LOG_KEY,
		fmt.Sprintf("mappedRequest %q: %s => %s (based on matching handler for %q",
			clientID, r.URL.Path, mappedRequest.URL.Path, matchingPattern))

	handler.ServeHTTP(w, &mappedRequest)
}

func writeError(code int, w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(code)
	fmt.Fprintf(w, "%d %q for %s\n\n", code, http.StatusText(code), r.URL.Path)
}

// copy of http.cleanPath(); we dont want mux.Handler() return a redirect-handler.
// see code of how (*http.ServeMux)(Handler(r *http.Request)) is implemented
func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	if p[0] != '/' {
		p = "/" + p
	}
	np := path.Clean(p)
	if p[len(p)-1] == '/' && np != "/" {
		np += "/"
	}
	return np
}
