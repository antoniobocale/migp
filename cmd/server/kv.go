// Copyright (c) 2021 Cloudflare, Inc. All rights reserved.
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"errors"
	"io"
	"log"
	"os"
	"strings"
	"sync"
)
import "encoding/json"

// kvStore is a wrapper for a KV store. For now just use a simple dynamically
// allocated in-memory go map This won't scale properly, but ok for testing.
// Implements migp.Getter
type kvStore struct {
	store map[string][]byte
	lock  sync.RWMutex
}

// newKVStore initializes a new bucket store. Just using a simple map for now.
func newKVStore() (*kvStore, error) {
	return &kvStore{
		store: make(map[string][]byte),
	}, nil
}

// Put a value at key id and replace any existing value.
func (kv *kvStore) Put(id string, value []byte) error {
	kv.lock.Lock()
	defer kv.lock.Unlock()
	kv.store[id] = value
	return nil
}

// Append a value to any existing value at key id.
func (kv *kvStore) Append(id string, value []byte) error {
	kv.lock.Lock()
	defer kv.lock.Unlock()
	kv.store[id] = append(kv.store[id], value...)
	return nil
}

// Get returns the value in the key identified by id.
func (kv *kvStore) Get(id string) ([]byte, error) {
	var path = strings.Join(strings.Split(id, ""), "/")
	path = path[:len(path)-1]
	bucket, err := kv.LoadBucket("./store_test/"+path+id, Bytes)
	if err != nil {
		return nil, nil
	}
	return bucket, nil
	/*kv.lock.RLock()
	defer kv.lock.RUnlock()
	return kv.store[id], nil*/
}

var lock sync.Mutex

// Marshal is a function that marshals the object into an
// io.Reader.
// By default, it uses the JSON marshaller.
var Marshal = func(v interface{}) (io.Reader, error) {
	b, err := json.MarshalIndent(v, "", "\t")
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}

func (kv *kvStore) saveCredentials() {
	//start := time.Now()
	if _, err := os.Stat("store_test"); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir("store_test", os.ModePerm)
		if err != nil {
			log.Println(err)
		}
	}
	for k, v := range kv.store {
		if err := kv.SaveBucket("./store_test/", k, v, Bytes); err != nil {
			log.Fatalln(err)
		}
	}
	//fmt.Printf("\r") ++++++++
	/*t := time.Now()
	elapsed := t.Sub(start)
	fmt.Printf("\rSaving took %s\n", elapsed)*/
}

type FileFormat int8

const (
	JSON FileFormat = iota
	Bytes
)

func (kv *kvStore) SaveBucket(root string, bucketID string, bucket []byte, fileFormat FileFormat) error {
	//lock.Lock()
	//defer lock.Unlock()
	//fmt.Printf("\rSaving bucket %s", bucketID) ++++++++
	var path = strings.Join(strings.Split(bucketID, ""), "/")
	path = path[:len(path)-1]

	if _, err := os.Stat(root + path + bucketID); os.IsNotExist(err) {
		//fmt.Printf("File does not exist\n")
		err := os.MkdirAll(root+path, os.ModePerm)
		if err != nil {
			return err
			//log.Fatalln(err)
		}
	} else if fileFormat == JSON {
		//fmt.Printf("\rFile exists: %s", root+path+bucketID)
		existingBucket, _ := kv.LoadBucket(root+path+bucketID, fileFormat)
		bucket = append(existingBucket, bucket...)
	}

	switch fileFormat {
	case Bytes:
		f, err := os.OpenFile(root+path+bucketID,
			os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
			//log.Println(err)
		}
		defer f.Close()
		if _, err := f.Write(bucket); err != nil {
			return err
			//log.Println(err)
		}
	case JSON:
		f, err := os.Create(root + path + bucketID)
		if err != nil {
			return err
		}
		defer f.Close()
		r, err := Marshal(bucket)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, r)
		return err
	}
	return nil
}

// Unmarshal is a function that unmarshals the data from the
// reader into the specified value.
// By default, it uses the JSON unmarshaller.
var Unmarshal = func(r io.Reader, v interface{}) error {
	return json.NewDecoder(r).Decode(v)
}

func (kv *kvStore) LoadBucket(bucketID string, fileFormat FileFormat) ([]byte, error) {
	lock.Lock()
	defer lock.Unlock()

	switch fileFormat {
	case Bytes:
		bucket, error := os.ReadFile(bucketID)
		if error != nil {
			//print("error loading bucket bytes")
			return nil, error
		}
		return bucket, nil
	case JSON:
		f, err := os.Open(bucketID)
		if err != nil {
			//log.Fatalln(err)
			return nil, err
		}
		defer f.Close()
		var bucket []byte
		err = Unmarshal(f, &bucket)
		if err != nil {
			//log.Fatalln(err)
			return nil, err
		}
		return bucket, nil
	}
	return nil, nil
}
