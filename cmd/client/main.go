// Copyright (c) 2021 Cloudflare, Inc. All rights reserved.
// SPDX-License-Identifier: BSD-3-Clause

// client implements the client-side logic to interact with an OPRF service and decrypt a bucket.

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/cloudflare/migp-go/pkg/migp"
)

func main() {
	var targetURL, configFile, inputFilename string
	var dumpConfig, showPassword bool
	var err error

	flag.StringVar(&configFile, "config", "", "Client configuration file (default: retrieve from server)")
	flag.BoolVar(&dumpConfig, "dump-config", false, "Dump the client configuration to stdout and exit")
	flag.BoolVar(&showPassword, "show-password", false, "Show the password in the output")
	flag.StringVar(&inputFilename, "infile", "-", "input file of credentials to query in the format <username>:<password> ('-' for stdin)")
	flag.StringVar(&targetURL, "target", "http://localhost:8080", "target MIGP server")

	flag.Parse()

	var cfg migp.Config
	if configFile != "" {
		// use the provided config file
		data, err := os.ReadFile(configFile)
		if err != nil {
			log.Fatal(err)
		}
		err = json.Unmarshal(data, &cfg)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		// retrieve the config from the server
		resp, err := http.Get(targetURL + "/config")
		if err != nil {
			log.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			log.Fatalf("Unable to retrieve MIGP config from target %q: status code %d", targetURL, resp.StatusCode)
		}
		decoder := json.NewDecoder(resp.Body)
		if err := decoder.Decode(&cfg); err != nil {
			log.Fatal(err)
		}
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

	if cfg.Version != migp.DefaultMIGPVersion {
		log.Printf("WARN: Your MIGP library version (%d) does not match the version specified in the config (%d) and may not be compatible.", migp.DefaultMIGPVersion, cfg.Version)
	}

	inputFile := os.Stdin
	if inputFilename != "-" {
		if inputFile, err = os.Open(inputFilename); err != nil {
			log.Fatal(err)
		}
		defer inputFile.Close()
	}

	scanner := bufio.NewScanner(inputFile)

	query_count := int64(0)
	bw := float64(0)
	query_prep := time.Duration(0)
	api_call := time.Duration(0)
	finalize := time.Duration(0)
	total := time.Duration(0)

	for scanner.Scan() {
		fields := bytes.SplitN(scanner.Bytes(), []byte(":"), 2)
		if len(fields) < 2 {
			continue
		}
		username, password := fields[0], fields[1]
		if status, metadata, err, duration, b := migp.Query(cfg, targetURL+"/evaluate", username, password); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		} else {
			query_count += 1
			bw += b
			query_prep += duration["query_prep"]
			api_call += duration["api_call"]
			finalize += duration["finalize"]
			total += duration["total"]

			if !showPassword {
				password = nil
			}
			out, err := json.Marshal(struct {
				Username string `json:"username"`
				Password string `json:"password,omitempty"`
				Status   string `json:"status"`
				Metadata string `json:"metadata,omitempty"`
			}{
				Username: string(username),
				Password: string(password),
				Status:   status.String(),
				Metadata: string(metadata),
			})
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			fmt.Println(string(out))
		}
	}
	fmt.Printf("Query count: %d\n", query_count)
	query_prep = time.Duration(query_prep.Nanoseconds() / query_count)
	api_call = time.Duration(api_call.Nanoseconds() / query_count)
	finalize = time.Duration(finalize.Nanoseconds() / query_count)
	total = time.Duration(total.Nanoseconds() / query_count)
	bw = bw / float64(query_count)
	//fmt.Println("------------AVG------------")
	fmt.Printf("Query Prep. %s\n", query_prep)
	fmt.Printf("API call %s\n", api_call)
	fmt.Printf("Finalize %s\n", finalize)
	fmt.Printf("Total %s\n", total)
	fmt.Printf("B/w (MB) %.2f\n", bw)
}
