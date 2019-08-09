// This application makes an HTTP or HTTPS request to one or more target URLs and
// reports detailed DNS, TCP, TLS, and first byte response times, along with overall
// application response time.  It can publish data to Cloudwatch, publish the details to
// a webhook, such as a StreamSets endpoint, or trigger alerts via Twilio.
// From https://github.com/davecheney/httpstat, from https://github.com/reorx/httpstat.
package main

import (
	cw "github.com/rafayopen/perftest/pkg/cw"
	pf "github.com/rafayopen/perftest/pkg/flag"
	pt "github.com/rafayopen/perftest/pkg/pt"
	srv "github.com/rafayopen/perftest/pkg/srv"

	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const usage = `Usage: %s [flags] URL ...
URLs to test -- there may be multiple of them, all will be tested in parallel.
Continue to issue requests every $delay seconds; if delay==0, make requests until interrupted.
Can stop after some number of cycles (-n), or when enough failures occur, or signaled to stop.

Can send an alert if desired if total response time is over a threshold.
Supported alerting mechanisms:
  - Twilio (requires account ID and API key in shell environment)

The app behavior is controlled via command line flags and environment variables.
See README.md for a description.

Command line flags:
`

var (
	// Location of perftest instance to be published to Cloudwatch
	myLocation string

	delayFlag     = flag.Int("d", 10, "delay in seconds between test requests")
	maxFails      = flag.Int("f", 10, "maximum number of failures before process quits")
	numTests      = flag.Int("n", 0, "number of tests to each endpoint (default 0 runs until interrupted)")
	jsonFlag      = flag.Bool("j", false, "write detailed metrics in JSON (default is text TSV format)")
	alertMsec     = flag.Int64("A", 0, "alert threshold in milliseconds")
	alertInterval = flag.Int64("M", 300, "minimum time interval between generated alerts (seconds)")
	cwFlag        = flag.Bool("c", false, "Publish metrics to CloudWatch (requires AWS credentials in env)")
	webhook       = flag.String("W", "", "Webhook target URL to receive JSON log details via POST")
	portFlag      = flag.Int("p", 0, "run web server on this port (if non-zero) to report stats")
	qf            = flag.Bool("q", false, "be quiet, not verbose")
	vf1           = flag.Bool("v", false, "be verbose")
	vf2           = flag.Bool("V", false, "be more verbose")

	whURL    string       // URL of webhook server
	whClient *http.Client // HTTP client object used for HTTP POST to webhook

	verbose = 1

	alertThresh time.Duration      // alert threshold value (from environment)
	twilioSms   pf.StringArrayFlag // array of Twilio SMS numbers to alert
	twilioKey   string             // holds Twilio accountSid:authToken
	smsSender   string             // SMS sender number registered -- must be with Twilio
)

func printUsage() {
	fmt.Fprintf(os.Stderr, usage, os.Args[0])
	flag.PrintDefaults()
}

func initializeHTTP() {
	// create Transport to carry requests to the SS endpoint
	// create Client to make POST requests to the SS endpoint
	// remember to set Connection: keep-alive

	ssTransport := &http.Transport{
		MaxIdleConnsPerHost: 10,
		TLSHandshakeTimeout: 10 * time.Second,
		Dial: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).Dial,
	}

	// be sure to set client timeout so it doesn't wait forever
	whClient = &http.Client{
		Transport: ssTransport,
		Timeout:   10 * time.Second,
	}
}

// publishJSON sends the PingTimes struct in JSON to the webhook endpoint url.
func publishJSON(url string, pt *pt.PingTimes) {
	jsonData, err := json.Marshal(pt)
	if err != nil {
		log.Println("failed to marshal PingTimes", err)
		return
	}

	// This will wait for the POST to complete before returning ...
	// no more perftest requests will happen until this is done ...
	// so maybe this should be a goroutine rather than inline?
	resp, err := whClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Println(err)
		// NOTE: May need to recreate whClient here, depending on the error
		return
		// If a transient error just ignore it, try again next time
	}

	io.Copy(ioutil.Discard, resp.Body)
	// must drain and close the response body for TCP/TLS connection reuse
	resp.Body.Close()
}

// Read command line arguments, take action, and report results to stdout.
func main() {
	flag.Usage = printUsage
	flag.Parse()

	if *qf {
		verbose = 0
	}
	if *vf1 {
		verbose++
	}
	if *vf2 {
		verbose += 2
	}

	whURL = os.Getenv("HTTP_JSON_WEBHOOK")
	if len(*webhook) > 0 {
		if len(whURL) > 0 {
			log.Println("NOTE: overwriting webhook from env,", whURL, "via command line")
		}
		whURL = *webhook
	}

	if len(whURL) > 0 {
		sslPrefix := "https://"
		if strings.HasPrefix(whURL, sslPrefix) {
			initializeHTTP() // initializes transport and whClient
		} else {
			log.Println("ERROR: webhook URL must start with", sslPrefix)
			// whClient remains nil, no data will be posted to it
		}
	}

	tas := os.Getenv("TWILIO_ACCOUNT_SID")
	tat := os.Getenv("TWILIO_AUTH_TOKEN")
	if len(tas) > 0 && len(tat) > 0 {
		twilioKey = tas + ":" + tat
	}

	if smslist, found := os.LookupEnv("TWILIO_SMS_RECEIVERS"); found {
		for _, sms := range strings.Split(smslist, " ") {
			twilioSms = append(twilioSms, sms)
		}
	}

	alertThresh = time.Duration(*alertMsec) * time.Millisecond
	if rt, found := os.LookupEnv("RESPONSE_THRESHOLD"); found {
		if *alertMsec > 0 {
			log.Println("NOTE: alert threshold from commandline overrides environment:", rt)
		} else {
			if at, err := strconv.Atoi(rt); err == nil {
				alertThresh = time.Duration(at) * time.Millisecond
			} else {
				log.Println("parsing environment var RESPONSE_THRESHOLD:", err)
			}
		}
	}

	if 0 == alertThresh {
		// set to an impossibly high value for a single request ...
		alertThresh = 24 * time.Hour
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

	myLocation = pt.LocationFromEnv()

	if *cwFlag {
		cwRegion := os.Getenv("AWS_REGION")
		if len(cwRegion) > 0 && len(os.Getenv("AWS_ACCESS_KEY_ID")) > 0 && len(os.Getenv("AWS_SECRET_ACCESS_KEY")) > 0 {
			log.Println("publishing to CloudWatch region", cwRegion)
		} else {
			log.Println("CloudWatch requires in environment: AWS_REGION, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY")
			*cwFlag = false
		}
	}

	if whClient != nil && verbose > 0 {
		log.Println("publishing to webhook", whURL)
	}

	if verbose > 0 {
		log.Println("testing ", urls, "from", pt.LocationOrIp(&myLocation))
	}

	serverPort := 0
	if portEnv, found := os.LookupEnv("PERFTEST_LISTEN_PORT"); found {
		val, err := strconv.Atoi(portEnv)
		if err != nil {
			log.Println("ERROR: cannot parse PERFTEST_LISTEN_PORT as an int:", err)
			return
		}
		serverPort = val
	}

	wasFlagPassed := func(fn string) bool {
		found := false
		flag.Visit(func(f *flag.Flag) {
			if f.Name == fn {
				found = true
			}
		})
		return found
	}

	if delayEnv, found := os.LookupEnv("PERFTEST_DELAY"); found {
		delay, err := strconv.Atoi(delayEnv)
		if err != nil || delay < 1 {
			log.Println("Warning: PERFTEST_DELAY environment is", delayEnv, "-- value must be int > 0.  Using -d", *delayFlag, "instead")
		} else {
			if wasFlagPassed("d") {
				log.Println("Note: PERFTEST_DELAY from environment,", delay, "overrides -d", *delayFlag)
			}
			*delayFlag = delay
		}
	}

	if numEnv, found := os.LookupEnv("PERFTEST_LIMIT"); found {
		num, err := strconv.Atoi(numEnv)
		if err != nil || num < 1 {
			log.Println("Warning: PERFTEST_LIMIT from environment is", numEnv, "-- value must be int > 0.  Using -n", *numTests, "instead")
		} else {
			if wasFlagPassed("n") {
				log.Println("Note: PERFTEST_LIMIT from environment,", num, "overrides -n", *numTests)
			}
			*numTests = num
		}
	}

	if *portFlag > 0 {
		if serverPort > 0 {
			log.Println("NOTE: command line port", *portFlag, "overrides listen port from env", serverPort)
		}
		serverPort = *portFlag
	}

	if serverPort > 0 {
		if serverPort > 65535 {
			log.Println("bad port > 65535", serverPort)
			return
		}

		if verbose > 0 {
			log.Printf("Starting responder on http://localhost:%d\n", serverPort)
		}
		go srv.StartServer(serverPort)
		// http.HandleFunc("/ping", srv.pongReply)
		http.HandleFunc("/memstats", srv.MemStatsReply)
	}

	////
	// Run testHTTP for each endpoint in a goroutine synchronized with a WaitGroup
	////

	var doneChan = make(chan int) // signals when testHTTP should stop testing
	wg := new(sync.WaitGroup)     // coordinates exit across goroutines

	// Set up signal handler to close down gracefully
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, os.Interrupt)
	signal.Notify(sigchan, syscall.SIGTERM)
	go func() {
		for sig := range sigchan {
			if verbose > 1 {
				fmt.Println("\nreceived", sig, "signal, terminating")
			}
			if doneChan != nil {
				close(doneChan)
				doneChan = nil
			}
			close(sigchan)
		}
	}()

	for _, url := range urls {
		wg.Add(1)                                 // wg.Add must finish before Wait()
		go testHTTP(url, *numTests, doneChan, wg) // will call wg.Done before it returns
	}

	// wait for group including ponger if Add(1) preceeds it ...
	if verbose > 1 {
		log.Println("waiting for children to exit")
	}

	if !*jsonFlag {
		// put header after any debug messages, but there's a race condition here :-)
		pt.TextHeader(os.Stdout)
	}

	wg.Wait()

	if verbose > 2 {
		log.Println("all tests exited, returning from main")
	}
	return // do not os.Exit, it will not run deferred (cleanup) functions ... (if any)
}

// testHTTP sends HTTP request(s) to the given URL and captures detailed timing information.
// It will repeat the request after a delay interval (in time.Seconds) elapses.
// It will make numTries attempts.
// It will exit if the done channel closes.
// Calls WaitGroup.Done upon return so caller knows when all work is finished.
func testHTTP(uri string, numTries int, done <-chan int, wg *sync.WaitGroup) {
	// clear this task in the waitgroup when returning
	defer wg.Done()
	if numTries == 0 {
		numTries = math.MaxInt32
	}

	url := pt.ParseURL(uri)
	urlStr := url.Scheme + "://" + url.Host + url.Path

	if verbose > 2 {
		log.Println("test", urlStr)
	}

	var enc *json.Encoder
	if *jsonFlag {
		enc = json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
	}

	var count int64            // successful
	failcount := 0             // failed
	var ptSummary pt.PingTimes // aggregates ping time results
	mn := "RespTime"           // CloudWatch metric name
	ns := "Http Perf Demo"     // CloudWatch namespace

	for {
		ptResult := pt.FetchURL(urlStr, myLocation)
		if nil == ptResult {
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
				ptSummary = *ptResult
				defer func() { // summary printer, runs upon return
					elapsed := hhmmss(time.Now().Unix() - ptSummary.Start.Unix())

					fc := float64(count) // count will be 1 by time this runs
					fmt.Printf("\nRecorded %d samples in %s, average values:\n"+"%s"+
						"%d %-6s\t%.03f\t%.03f\t%.03f\t%.03f\t%.03f\t%.03f\t\t%d\t%s\t%s\n\n",
						count, elapsed, pt.PingTimesHeader(),
						count, elapsed,
						pt.Msec(ptSummary.DnsLk)/fc,
						pt.Msec(ptSummary.TcpHs)/fc,
						pt.Msec(ptSummary.TlsHs)/fc,
						pt.Msec(ptSummary.Reply)/fc,
						pt.Msec(ptSummary.Close)/fc,
						pt.Msec(ptSummary.RespTime())/fc,
						// TODO: report summary stats per response code
						ptSummary.Size/count,
						"", // TODO: report summary of each from location?
						*ptSummary.DestUrl)
				}()
				// TODO: report all summaries just before program exit (not thread exit)
			} else {
				ptSummary.DnsLk += ptResult.DnsLk
				ptSummary.TcpHs += ptResult.TcpHs
				ptSummary.TlsHs += ptResult.TlsHs
				ptSummary.Reply += ptResult.Reply
				ptSummary.Close += ptResult.Close
				ptSummary.Total += ptResult.Total
				ptSummary.Size += ptResult.Size
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
				enc.Encode(ptResult)
			} else {
				fmt.Println(count, ptResult.MsecTsv())
			}

			if *cwFlag {
				if verbose > 1 {
					log.Println("publishing", pt.Msec(ptResult.RespTime()), "msec to cloudwatch")
				}
				respCode := "0"
				if ptResult.RespCode >= 0 {
					// 000 in cloudwatch indicates it was a zero return code from lower layer
					// while single digit 0 indicates an error making the request
					respCode = fmt.Sprintf("%03d", ptResult.RespCode)
				}

				cw.PublishRespTime(myLocation, urlStr, respCode, pt.Msec(ptResult.RespTime()), mn, ns)
			}

			if whClient != nil {
				if verbose > 1 {
					log.Println("publishing", ptResult.Remote, "to webhook")
				}
				publishJSON(whURL, ptResult)
			}

			// check if respose time exceeds threshold
			if ptResult.RespTime() > alertThresh {
				// generate any requested alerts
				sendAlert(ptResult, urlStr)
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

////////////////////////////////////////////////////////////////////////////////////////
//  Alert management
////////////////////////////////////////////////////////////////////////////////////////

// Unix time of last alert ... to compare to
var lastAlert int64

func sendAlert(ptResult *pt.PingTimes, url string) {
	timeSinceLast := ptResult.Start.Unix() - lastAlert
	msg := fmt.Sprintf("RespTime %s on %s exceeds %s", ptResult.RespTime(), url, alertThresh)
	if verbose > 0 {
		log.Println(msg)
	}

	if timeSinceLast < *alertInterval {
		if verbose > 1 {
			log.Println("too soon to send another alert")
		}
		return
	}
	lastAlert = ptResult.Start.Unix()

	if 0 == len(twilioKey) || 0 == len(twilioSms) {
		log.Println("OOPS: nowhere to send notification for", url)
	} else {
		for _, sms := range twilioSms {
			sendTwilio(msg, twilioKey, sms)
		}
	}
}

func sendTwilio(msg, key, sms string) {
	separator := strings.Index(key, ":")
	if -1 == separator {
		log.Println("incorrect formation for Twilio account:token")
		return
	}
	accountSid := key[:separator]
	authToken := key[1+separator:]

	twilioURL := "https://api.twilio.com/2010-04-01/Accounts/" + accountSid + "/Messages.json"

	if verbose > 1 {
		log.Println("sending Twilio msg to SMS", sms)
	}
	// Pack up the data for our message
	msgData := url.Values{}
	msgData.Set("To", sms)
	msgData.Set("From", smsSender)
	msgData.Set("Body", msg)
	msgDataReader := *strings.NewReader(msgData.Encode())

	// Create HTTP request client
	client := &http.Client{}
	req, _ := http.NewRequest("POST", twilioURL, &msgDataReader)
	req.SetBasicAuth(accountSid, authToken)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// Make HTTP POST request and return message SID
	resp, _ := client.Do(req)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var data map[string]interface{}
		decoder := json.NewDecoder(resp.Body)
		err := decoder.Decode(&data)
		if err == nil {
			fmt.Println(data["sid"])
		}
	} else {
		log.Println("HTTP error", resp.Status)
	}
}
