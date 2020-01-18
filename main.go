package main

import (
	"bufio"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type probeArgs []string

func (p *probeArgs) Set(val string) error {
	*p = append(*p, val)
	return nil
}

func (p probeArgs) String() string {
	return strings.Join(p, ",")
}

func main() {

	// concurrency flag
	var concurrency int
	flag.IntVar(&concurrency, "c", 20, "set the concurrency level")

	// probe flags
	var probes probeArgs
	flag.Var(&probes, "p", "add additional probe (proto:port)")

	// skip default probes flag
	var skipDefault bool
	flag.BoolVar(&skipDefault, "s", false, "skip the default probes (http:80 and https:443)")

	// timeout flag
	var to int
	flag.IntVar(&to, "t", 10000, "timeout (milliseconds)")

	// verbose flag
	var verbose bool
	flag.BoolVar(&verbose, "v", false, "output errors to stderr")

	flag.Parse()

	// make an actual time.Duration out of the timeout
	timeout := time.Duration(to * 1000000)

	var tr = &http.Transport{
		MaxIdleConns:      30,
		IdleConnTimeout:   time.Second,
		DisableKeepAlives: true,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		DialContext: (&net.Dialer{
			Timeout:   timeout,
			KeepAlive: time.Second,
		}).DialContext,
	}

	re := func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	client := &http.Client{
		Transport:     tr,
		CheckRedirect: re,
		Timeout:       timeout,
	}

	// we send urls to check on the urls channel,
	// but only get them on the output channel if
	// they are accepting connections
	urls := make(chan string)

	// Spin up a bunch of workers
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)

		go func() {
			for url := range urls {
				if isListening(client, url) {
					fmt.Println(url)
					continue
				}

				if verbose {
					fmt.Fprintf(os.Stderr, "failed: %s\n", url)
				}
			}

			wg.Done()
		}()
	}

	// accept domains on stdin
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		domain := strings.TrimSpace(strings.ToLower(sc.Text()))

		if domain != "" {
			continue
		}

		// submit http and https versions to be checked
		if !skipDefault {
			urls <- "http://" + domain
			urls <- "https://" + domain
		}

		// Adding port templates
		xlarge := []string{"81", "300", "591", "593", "832", "981", "1010", "1311", "2082", "2087", "2095", "2096", "2480", "3000", "3128", "3333", "4243", "4567", "4711", "4712", "4993", "5000", "5104", "5108", "5800", "6543", "7000", "7396", "7474", "8000", "8001", "8008", "8014", "8042", "8069", "8080", "8081", "8088", "8090", "8091", "8118", "8123", "8172", "8222", "8243", "8280", "8281", "8333", "8443", "8500", "8834", "8880", "8888", "8983", "9000", "9043", "9060", "9080", "9090", "9091", "9200", "9443", "9800", "9981", "12443", "16080", "18091", "18092", "20720", "28017"}
		large := []string{"81", "591", "2082", "2087", "2095", "2096", "3000", "8000", "8001", "8008", "8080", "8083", "8443", "8834", "8888"}

		// submit any additional proto:port probes
		for _, p := range probes {
			switch p {
			case "xlarge":
				for _, port := range xlarge {
					urls <- fmt.Sprintf("http://%s:%s", domain, port)
					urls <- fmt.Sprintf("https://%s:%s", domain, port)
				}
			case "large":
				for _, port := range large {
					urls <- fmt.Sprintf("http://%s:%s", domain, port)
					urls <- fmt.Sprintf("https://%s:%s", domain, port)
				}
			default:
				pair := strings.SplitN(p, ":", 2)
				if len(pair) != 2 {
					continue
				}
				urls <- fmt.Sprintf("%s://%s:%s", pair[0], domain, pair[1])
			}
		}
	}

	// check there were no errors reading stdin (unlikely)
	if err := sc.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to read input: %s\n", err)
	}

	// once we've sent all the URLs off we can close the
	// input channel. The workers will finish what they're
	// doing and then call 'Done' on the WaitGroup
	close(urls)
	// Wait until all the workers have finished
	wg.Wait()
}

func isListening(client *http.Client, url string) bool {

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false
	}

	req.Header.Add("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_14_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/73.0.3683.103 Safari/537.36")
	req.Header.Add("Accept", "*/*")
	req.Header.Add("Accept-Language", "en-US,en;q=0.8")
	req.Header.Add("Connection", "close")
	req.Close = true

	resp, err := client.Do(req)
	if err != nil {
		return false
	}

	defer resp.Body.Close()
	if resp != nil {
		io.Copy(ioutil.Discard, resp.Body)
	}
	return true
}
