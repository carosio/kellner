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
	clientId := clientIdByName(&cert.Subject)

	// TODO: do we want this?
	w.Header().Set("Kellner-Client-Id", clientId)

	// TODO: decide how we treat "/" requests ...
	// we could deliver opkg.conf containing all the valid repos
	// or something like that?
	if r.URL.Path == "/" {
		writeError(http.StatusForbidden, w, r)
		return
	}

	// TODO: optionally: try to be less and less specific by cutting
	// elements from []cert.Subject.Names

	clientDir := filepath.Join(muxer.IdRoot, clientId)

	requestedPath := path.Clean(r.URL.Path)
	mapFile := filepath.Join(clientDir, requestedPath)
	// try different map-files
	fi, err := os.Lstat(mapFile)
	if err != nil {
		requestedPath = path.Dir(requestedPath)
		mapFile = filepath.Join(clientDir, requestedPath)
		fi, err = os.Lstat(mapFile)
		if err != nil {
			writeError(http.StatusForbidden, w, r)
			return
		}
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
			clientDir, r.URL.Path, mappedRequest.URL.Path, matchingPattern))

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
