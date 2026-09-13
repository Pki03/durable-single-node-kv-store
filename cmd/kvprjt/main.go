package main

import (
	"flag"
	"fmt"
	"log"
	"sync"

	"github.com/prateekkhurmi/kvprjt/store"
)

var (
	dir     = flag.String("dir", "./data", "store directory")
	seed    = flag.String("seed", "", "seed for test data generation")
	count   = flag.Int("count", 0, "number of records to write (0=interactive)")
	recover = flag.Bool("recover", false, "run recovery only")
	verify  = flag.Bool("verify", false, "verify keys exist after recovery")
	verifiedCount = flag.Int("verified", 0, "number of confirmed writes to verify")
)

func main() {
	flag.Parse()

	if *recover {
		s, err := store.New(*dir)
		if err != nil {
			log.Fatalf("recovery failed: %v", err)
		}
		fmt.Printf("RECOVERED:%d\n", s.Size())
		s.Close()
		return
	}

	if *verify {
		s, err := store.New(*dir)
		if err != nil {
			log.Fatalf("verify failed: %v", err)
		}
		defer s.Close()
		missing := 0
		checkCount := *count
		if *verifiedCount > 0 {
			checkCount = *verifiedCount
		}
		for i := 0; i < checkCount; i++ {
			key := fmt.Sprintf("key_%06d", i)
			_, err := s.Get(key)
			if err != nil {
				missing++
			}
		}
		fmt.Printf("VERIFY:%d:%d\n", checkCount, missing)
		return
	}

	s, err := store.New(*dir)
	if err != nil {
		log.Fatalf("store creation failed: %v", err)
	}
	defer s.Close()

	if *count > 0 {
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < *count; i++ {
				key := fmt.Sprintf("key_%06d", i)
				val := fmt.Sprintf("value_%06d", i)
				if err := s.Put(key, []byte(val)); err != nil {
					log.Fatalf("put failed: %v", err)
				}
				fmt.Printf("OK:%s\n", key)
			}
		}()
		wg.Wait()
	} else {
		runInteractive(s)
	}
}

func runInteractive(s *store.Store) {
	fmt.Println("kvprjt interactive mode. Commands: put <key> <val>, get <key>, del <key>, size, exit")
	var cmd, key, val string
	for {
		fmt.Print("> ")
		_, err := fmt.Scan(&cmd)
		if err != nil {
			break
		}
		switch cmd {
		case "put":
			fmt.Scan(&key, &val)
			s.Put(key, []byte(val))
			fmt.Println("ok")
		case "get":
			v, err := s.Get(key)
			if err != nil {
				fmt.Println("not found")
			} else {
				fmt.Println(string(v))
			}
		case "del":
			s.Delete(key)
			fmt.Println("ok")
		case "size":
			fmt.Printf("size: %d\n", s.Size())
		case "exit":
			return
		}
	}
}
