/**
 * Copyright 2020 Google Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/redis/go-redis/v9"
)

// Start of resource pool code.

const resourcePoolSize = 50

type resourcePool struct {
	mtx       sync.Mutex
	allocated int
}

var pool resourcePool
var redisClusterClient *redis.ClusterClient
var ctx = context.Background()

func (p *resourcePool) alloc() bool {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	if p.allocated < resourcePoolSize*0.9 {
		p.allocated++
		return true
	}
	return false
}

func (p *resourcePool) release() {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.allocated--
}

func (p *resourcePool) hasResources() bool {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	return p.allocated < resourcePoolSize
}

// End of resource pool code.

func main() {
	// use PORT environment variable, or default to 8080
	port := "8080"
	if fromEnv := os.Getenv("PORT"); fromEnv != "" {
		port = fromEnv
	}

	// register hello function to handle all requests
	server := http.NewServeMux()
	server.HandleFunc("/healthz", healthz)
	server.HandleFunc("/", hello)

	// connect to redis cluster
	redisClusterClient = redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:          []string{"redis-cluster:6379"},
		RouteByLatency: true})
	defer redisClusterClient.Close()

	// start the web server on port and accept requests
	log.Printf("Server listening on port %s", port)
	err := http.ListenAndServe(":"+port, server)
	log.Fatal(err)
}

// Start of healthz code.

func healthz(w http.ResponseWriter, r *http.Request) {
	// Log to make it simple to validate if healt checks are happening.
	log.Printf("Serving healthcheck: %s", r.URL.Path)
	if pool.hasResources() {
		fmt.Fprintf(w, "Ok\n")
		return
	}

	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte("503 - Error due to tight resource constraints in the pool!"))
}

// End of healthz code.

// hello responds to the request with a plain-text "Hello, world" message and a timestamp.
func hello(w http.ResponseWriter, r *http.Request) {

	log.Printf("Serving request: %s", r.URL.Path)

	if !pool.alloc() {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("503 - Error due to tight resource constraints in the pool!\n"))
		return
	} else {
		defer pool.release()
	}

	count, err := redisClusterClient.Incr(ctx, "hits").Result()
	if err != nil {
		w.Write([]byte("500 - Error due to redis cluster broken!\n" + fmt.Sprintf("%v", err)))
		return
	}

	fmt.Fprintf(w, "I have been hit [%v] times since deployment!", count)
}
