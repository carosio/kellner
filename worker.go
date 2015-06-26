// This file is part of *kellner*
//
// Copyright (C) 2015, Travelping GmbH <copyright@travelping.com>
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import "sync"

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
