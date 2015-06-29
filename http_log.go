// This file is part of *kellner*
//
// Copyright (C) 2015, Travelping GmbH <copyright@travelping.com>
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"log"
	"net/http"
)

const _ExtraLogKey = "kellner-log-data"

// wraps 'orig_handler' to log incoming http-request
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		r.Header.Del(_ExtraLogKey)

		statusLog := logStatusCode{ResponseWriter: w}
		next.ServeHTTP(&statusLog, r)
		if statusLog.Code == 0 {
			statusLog.Code = 200
		}

		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			log.Println(r.RemoteAddr, r.Method, statusLog.Code, r.Host, r.RequestURI, r.Header)
			return
		}

		clientID := clientIDByName(&r.TLS.PeerCertificates[0].Subject)
		log.Println(r.RemoteAddr, clientID, r.Method, statusLog.Code, r.Host, r.RequestURI, r.Header)
	})
}

//
// small helper to intercept the http-statuscode written
// to the original http.ResponseWriter
type logStatusCode struct {
	http.ResponseWriter
	Code int
}

func (w *logStatusCode) WriteHeader(code int) {
	w.Code = code
	w.ResponseWriter.WriteHeader(code)
}
