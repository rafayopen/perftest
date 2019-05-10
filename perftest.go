// This application makes an HTTP or HTTPS request to one or more target URLs and
// reports detailed DNS, TCP, TLS, and first byte response times, along with overall
// application response time.
// From https://github.com/davecheney/httpstat, from https://github.com/reorx/httpstat.

package main

import (
	"github.com/rafayopen/perftest/util"

	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const usage = `Usage: %s [flags] URL ...
URLs to test -- there may be multiple of them, all will be tested in parallel.
Continue to issue requests every $delay seconds; if delay==0, make requests until interrupted.
Can stop after some number of cycles (-n), or when enough failures occur, or signaled to stop.

The app behavior is controlled via command line flags and environment variables.
See README.md for a description.

Command line flags:
`

var (
	// Location of perftest instance to be published to Cloudwatch
	myLocation string

	delayFlag = flag.Int("d", 10, "delay in seconds between test requests")
	maxFails  = flag.Int("f", 10, "maximum number of failures before process quits")
	numTests  = flag.Int("n", 0, "number of tests to each endpoint (default 0 means forever)")
	jsonFlag  = flag.Bool("j", false, "write detailed metrics in JSON (default is text TSV format)")

	qf  = flag.Bool("q", false, "be quiet, not verbose")
	vf1 = flag.Bool("v", false, "be verbose")
	vf2 = flag.Bool("V", false, "be more verbose")

	verbose = 0
)

func printUsage() {
	fmt.Fprintf(os.Stderr, usage, os.Args[0])
	flag.PrintDefaults()
}

// Read command line arguments, take action, and report results to stdout.
func main() {
	flag.Usage = printUsage
	flag.Parse()

	if *qf {
		verbose = 0
	}
	if *vf1 {
		verbose += 1
	}
	if *vf2 {
		verbose += 2
	}

	urls := flag.Args()
	if urlEnv, found := os.LookupEnv("PERFTEST_URL"); found {
		for _, url := range strings.Split(urlEnv, " ") {
			urls = append(urls, url)
		}
	}

	if len(urls) == 0 {
		log.Println("Error: no destinations to test")
		printUsage()
		os.Exit(1)
		// Do Not use os.Exit after this point (see return at end of main)
	}

	myLocation = util.LocationFromEnv()

	if verbose > 0 {
		log.Println("testing ", urls, "from", util.LocationOrIp(&myLocation))
	}

	if !*jsonFlag {
		util.TextHeader(os.Stdout)
	}

	////
	// Run testHttp for each endpoint in a goroutine synchronized with a WaitGroup
	////

	var doneChan = make(chan int) // signals when testHttp should stop testing
	wg := new(sync.WaitGroup)     // coordinates exit across goroutines

	// Set up signal handler to close down gracefully
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, os.Interrupt)
	signal.Notify(sigchan, syscall.SIGTERM)
	go func() {
		for sig := range sigchan {
			fmt.Println("\nreceived", sig, "signal, terminating")
			if doneChan != nil {
				close(doneChan)
				doneChan = nil
			}
		}
	}()

	for _, url := range urls {
		wg.Add(1)                                 // wg.Add must finish before Wait()
		go testHttp(url, *numTests, doneChan, wg) // will call wg.Done before it returns
	}

	// wait for group including ponger if Add(1) preceeds it ...
	if verbose > 1 {
		log.Println("waiting for children to exit")
	}
	wg.Wait()

	if verbose > 2 {
		log.Println("all tests exited, returning from main")
	}
	return // do not os.Exit, it will not run deferred (cleanup) functions ... (if any)
}

// testHttp sends HTTP request(s) to the given URL and captures detailed timing information.
// It will repeat the request after a delay interval (in time.Seconds) elapses.
// It will make numTries attempts.
// It will exit if the done channel closes.
// Calls WaitGroup.Done upon return so caller knows when all work is finished.
func testHttp(uri string, numTries int, done <-chan int, wg *sync.WaitGroup) {
	// clear this task in the waitgroup when returning
	defer wg.Done()
	if numTries == 0 {
		numTries = math.MaxInt32
	}

	url := util.ParseURL(uri)
	urlStr := url.Scheme + "://" + url.Host + url.Path

	if verbose > 2 {
		log.Println("test", urlStr)
	}

	var enc *json.Encoder
	if *jsonFlag {
		enc = json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
	}

	var count int64              // successful
	failcount := 0               // failed
	var ptSummary util.PingTimes // aggregates ping time results

	for {
		pt := util.FetchURL(urlStr, myLocation)
		if nil == pt {
			failcount++
			if failcount >= *maxFails {
				log.Println("fetch failure", failcount, "of", *maxFails, "on", url)
				// deferred routine below will print summary report if count > 0
				if count == 0 {
					fmt.Println("No valid samples received, no summary provided")
				}
				return
			}
			// fall out below, check done channel and try again after delay
		} else {
			if count == 0 {
				ptSummary = *pt
				defer func() { // summary printer, runs upon return
					elapsed := hhmmss(time.Now().Unix() - ptSummary.Start.Unix())

					fmt.Printf("\nRecorded %d samples in %s, average values:\n",
						count, elapsed)
					fc := float64(count) // count will be 1 by time this runs
					util.TextHeader(os.Stdout)
					fmt.Printf("%d %-6s\t%.03f\t%.03f\t%.03f\t%.03f\t%.03f\t%.03f\t\t%d\t%s\t%s\n\n",
						count, elapsed,
						util.Msec(ptSummary.DnsLk)/fc,
						util.Msec(ptSummary.TcpHs)/fc,
						util.Msec(ptSummary.TlsHs)/fc,
						util.Msec(ptSummary.Reply)/fc,
						util.Msec(ptSummary.Close)/fc,
						util.Msec(ptSummary.RespTime())/fc,
						// TODO: report summary stats per response code
						ptSummary.Size/count,
						"", // TODO: report summary of each from location?
						*ptSummary.DestUrl)
				}()
			} else {
				ptSummary.DnsLk += pt.DnsLk
				ptSummary.TcpHs += pt.TcpHs
				ptSummary.TlsHs += pt.TlsHs
				ptSummary.Reply += pt.Reply
				ptSummary.Close += pt.Close
				ptSummary.Total += pt.Total
				ptSummary.Size += pt.Size
				// TODO: record changes in Remote Server IP from DNS resolution
				// TODO: record count of different RespCode HTTP response code seen
				// or keep a summary object in a hash by unique RespCode
				// (in which case the count is needed in each one)
			}
			count++

			////
			//  Print out result of this test
			////
			if *jsonFlag {
				enc.Encode(pt)
			} else {
				fmt.Println(count, pt.MsecTsv())
			}
		}

		if count >= int64(numTries) {
			// report stats (see deferred func() above) upon return
			return
		}

		select {
		case <-done:
			// channel is closed, we are done -- report statistics and return
			return

		case <-time.After(time.Duration(*delayFlag) * time.Second):
			// we waited for the duration and the done channel is still open ... keep going
		}
	} // for ever
}

func hhmmss(secs int64) string {
	hr := secs / 3600
	secs -= hr * 3600
	min := secs / 60
	secs -= min * 60

	if hr > 0 {
		return fmt.Sprintf("%dh%02dm%02ds", hr, min, secs)
	}
	if min > 0 {
		return fmt.Sprintf("%dm%02ds", min, secs)
	}
	return fmt.Sprintf("%ds", secs)
}
