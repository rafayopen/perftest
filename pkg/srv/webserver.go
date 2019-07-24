package srv

import (
	"fmt"
	"log"
	"net/http"
	"runtime"
	"time"
)

// StartServer is a goroutine that runs up a web server to listen on the given
// port, and sets up handlers for two endpoints: "/ping" serves a simple HTTP
// reply that includes the request URI and current time; "/memstats" serves a
// simple HTTP page showing current golang memory utilization.  The port
// argument should be a valid port number that you can listen on.
//
// StartServer never returns.  Invoke it with go StartServer(yourPort).
func StartServer(port int) {
	max := 5 // 5 tries = 15 seconds (linear backoff -- 5th triangular number)

	addr := fmt.Sprintf(":%d", port)
	// The ListenAndServe call should not return.  If it does the address may be in use so retry below.
	err := http.ListenAndServe(addr, nil)

	tries := 0
	for tries < max {
		tries++
		// sleep a little while (longer each time through the loop)
		log.Println(err, "sleep", tries)
		time.Sleep(time.Duration(tries) * time.Second)
		// now try again ... it may take a while for a previous instance to exit
		err = http.ListenAndServe(addr, nil)
	}

	if err != nil {
		// if error is anything else print the error and return
		log.Println(err)
	}
}

// MemStatsReply returns a web page containing memory usage statistics
func MemStatsReply(w http.ResponseWriter, r *http.Request) {
	// with thanks from https://golangcode.com/print-the-current-memory-usage/
	var m runtime.MemStats

	bToMb := func(b uint64) uint64 {
		return b / 1024 / 1024
	}

	runtime.ReadMemStats(&m)
	active := m.Mallocs - m.Frees

	alloc := bToMb(m.Alloc)
	fmt.Fprintln(w, "<h1>MemStats</h1>\n<p>", time.Now())
	// The number of live objects is Mallocs - Frees.
	fmt.Fprintf(w, "<br>Active Objects = %v\n", active)
	// HeapAlloc is bytes of allocated heap objects.
	fmt.Fprintf(w, "<br>Active Bytes = %v MiB\n", alloc)
	// Sys is the total bytes of memory obtained from the OS.
	fmt.Fprintf(w, "<br>OS Bytes = %v MiB\n", bToMb(m.Sys))
	// NumGC is the number of completed GC cycles.
	fmt.Fprintf(w, "<br>Num GCs = %v\n</p>", m.NumGC)

	log.Println("pongReply", r.RemoteAddr, active, alloc)
}
