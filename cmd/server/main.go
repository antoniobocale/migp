// Copyright (c) 2021 Cloudflare, Inc. All rights reserved.
// SPDX-License-Identifier: BSD-3-Clause

// server implements a MIGP server. It supports encrypting and uploading a
// database of breach entries to buckets, and serving those buckets to clients
// via the MIGP protocol.

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/cloudflare/migp-go/pkg/migp"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func main() {

	var MEAN = make(map[int]float64)
	MEAN[16] = 1431876
	MEAN[20] = 89492

	var configFile, inputFilename, inputDirname, metadata, listenAddr string
	var dumpConfig, includeUsernameVariant bool
	var numVariants int
	var restore, start, test bool

	flag.StringVar(&configFile, "config", "", "Server configuration file")
	flag.StringVar(&listenAddr, "listen", "localhost:8080", "Server listen address")
	flag.BoolVar(&dumpConfig, "dump-config", false, "Dump the server configuration to stdout and exit")
	flag.StringVar(&inputFilename, "infile", "-", "input file of credentials to insert in the format <username>:<password> ('-' for stdin)")
	flag.StringVar(&inputDirname, "indir", "", "input directory of credentials to insert in the format <username>:<password>")
	flag.StringVar(&metadata, "metadata", "", "optional metadata string to store alongside breach entries")
	flag.IntVar(&numVariants, "num-variants", 9, "number of password variants to include")
	flag.BoolVar(&includeUsernameVariant, "username-variant", true, "include a username-only variant")
	flag.BoolVar(&start, "start", false, "start MIGP server without loading breach dataset")
	flag.BoolVar(&test, "test", false, "Get breach dataset info")

	flag.Parse()

	var cfg migp.ServerConfig
	if configFile != "" {
		data, err := os.ReadFile(configFile)
		if err != nil {
			log.Fatal(err)
		}
		err = json.Unmarshal(data, &cfg)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		cfg = migp.DefaultServerConfig()
	}

	if dumpConfig {
		data, err := json.Marshal(&cfg)
		if err != nil {
			log.Fatal(err)
		}
		_, err = os.Stdout.Write(data)
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	s, err := newServer(cfg)
	if err != nil {
		log.Fatal(err)
	}

	if start {
		log.Printf("\nStarting MIGP server")
		log.Fatal(http.ListenAndServe(listenAddr, s.handler()))
		return
	}

	if test {
		start := time.Now()
		numOfBuckets, numOfCredentials, avg, std := avgBucketSize(s, s.kv)
		t := time.Now()
		elapsed := t.Sub(start)
		fmt.Printf("Operation took %s\n", elapsed)
		fmt.Printf("#Buckets: %d\n", numOfBuckets)
		fmt.Printf("#Credentials: %d\n", numOfCredentials)
		fmt.Printf("Avg: %d\n", avg)
		fmt.Printf("Std: %d\n", std)
		log.Printf("\nStarting MIGP server")
		log.Fatal(http.ListenAndServe(listenAddr, s.handler()))
		return
	}

	if inputDirname != "" {
		var encryptionTime time.Duration = 0
		var savingTime time.Duration = 0

		filepath.Walk(inputDirname, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				log.Fatalf(err.Error())
			}

			if !info.IsDir() && info.Name()[0:1] != "." {
				fmt.Println(path)
				start := time.Now()
				s.processCredentials(path, metadata, numVariants, includeUsernameVariant)
				t := time.Now()
				//println() ++++++++
				elapsed := t.Sub(start)
				encryptionTime = encryptionTime + elapsed
				//var finished = path + " - " + encryptionTime.String()
				//fmt.Println(finished)
				//fmt.Println(strings.Repeat("-", len(finished)))
				t2 := time.Now()
				s.kv.saveCredentials()
				savingTime += time.Now().Sub(t2)
				kv, err := newKVStore()
				if err != nil {
					return err
				}
				s.kv = kv
			}
			return nil
		})
		fmt.Printf("\rEncryption took %s\n", encryptionTime)
		fmt.Printf("\rSaving took %s\n", savingTime)
	} else if inputFilename != "" {
		start := time.Now()
		s.processCredentials(inputFilename, metadata, numVariants, includeUsernameVariant)
		t := time.Now()
		elapsed := t.Sub(start)
		fmt.Printf("\n")
		s.kv.saveCredentials()
		for k, v := range s.kv.store {
			fmt.Printf("KV %s: %d bytes\n", k, len(v))
		}
		fmt.Printf("Encryption took %s\n", elapsed)
	}

}

func avgBucketSize(s *server, kv *kvStore) (int, int, int, int) {
	var numOfBuckets = 0
	var sizeOfBuckets []int
	var numOfCredentials = 0
	filepath.Walk("./store_test", func(path string, info os.FileInfo, err error) error {
		fmt.Println(path)
		if err != nil {
			log.Fatalf(err.Error())
		}
		if !info.IsDir() && info.Name()[0:1] != "." {
			bucket, _ := kv.LoadBucket(path, Bytes)
			if len(bucket) > 0 {
				numOfBuckets += 1
				sizeOfBuckets = append(sizeOfBuckets, len(bucket)/25)
				numOfCredentials = numOfCredentials + len(bucket)
			}
		}
		return nil
	})
	println()
	numOfCredentials = numOfCredentials / 25

	var numCred = numOfCredentials

	var avg = numOfCredentials / numOfBuckets

	for i, _ := range sizeOfBuckets {
		sizeOfBuckets[i] = sizeOfBuckets[i] - avg
		sizeOfBuckets[i] = sizeOfBuckets[i] * sizeOfBuckets[i]
	}

	numOfCredentials = 0
	for i := 0; i < len(sizeOfBuckets); i++ {
		numOfCredentials = numOfCredentials + sizeOfBuckets[i]
	}

	var std = math.Sqrt(float64(numOfCredentials) / float64(numOfBuckets))
	return numOfBuckets, numCred, avg, int(std)
}

func (s *server) processCredentials(file string, metadata string, numVariants int, includeUsernameVariant bool) {
	var err error
	inputFile := os.Stdin
	if file != "-" {
		if inputFile, err = os.Open(file); err != nil {
			log.Fatal(err)
		}
		defer inputFile.Close()
	}

	successCount, failureCount := 0, 0
	//fmt.Println(file)
	//log.Printf("Encrypting breach entries: %d successes, %d failures", successCount, failureCount)
	scanner := bufio.NewScanner(inputFile)
	for scanner.Scan() {
		fields := bytes.SplitN(scanner.Bytes(), []byte(":"), 2)
		if len(fields) < 2 {
			failureCount += 1
			continue
		}
		username, password := fields[0], fields[1]
		if err := s.insert(username, password, []byte(metadata), numVariants, includeUsernameVariant); err != nil {
			failureCount += 1
			continue
		}
		successCount += 1
		//fmt.Printf("\rEncrypting breach entries: %d successes, %d failures", successCount, failureCount) ++++++++
	}
}
