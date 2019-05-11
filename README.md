# Rafay Performance Monitoring Application #

This repo lets you build and run two of three flavors of a performance measurement and monitoring
application.  This is the first version of the application, demonstrating minimal functionality.
The next version will build out more functionality.

1.  Standalone -- a golang app you can run on your laptop.
2.  Docker -- a container that you can run as a component in many locations.
3.  Rafay Workload -- the Docker container running in one or more Edge locations on the Rafay
    platform.  (Coming in the full version of the app.)


## Description

This application makes an HTTP or HTTPS request to one or more target URLs and collects overall
response time data as well as detailed response time statistics.

### Prerequisites

Perftest is written in [golang](https://golang.org/doc/install).
Install and configure it using instructions at the link above.

If you don't want to install `go`, you can run a prebuilt version of
`perftest` from DockerHub (see "Run from Docker" below).

If you want to build and run perftest in a container you will need the
[Docker environment](https://docs.docker.com/get-started/).  If you have
golang you can still build and run a standalone version.

## How to Build and Run

Clone this repo.
  * Use "gmake standalone" to build the local standalone app.
  * Use "gmake docker" if you want to build the docker image.
  

Once you've built the app you can run it in one of two ways, standalone or as a docker.

**Standalone**: To run a test from the command line: `./perftest -n 5 https://www.google.com`.  You
will see output like this:

    # timestamp	DNS	TCP	TLS	First	LastB	Total	HTTP	Size	From_Location	Remote_Addr	proto://uri
    1 1554917703	24.168	14.607	127.732	61.524	1.333	209.282	200	12051	192.168.2.35	172.217.0.36	https://www.google.com
    2 1554917713	1.374	14.204	49.462	59.318	1.995	125.206	200	12017	192.168.2.35	172.217.0.36	https://www.google.com
    3 1554917723	1.265	14.341	52.774	63.336	3.908	134.661	200	12052	192.168.2.35	172.217.0.36	https://www.google.com
    4 1554917733	2.007	17.288	56.195	65.746	1.727	141.187	200	12000	192.168.2.35	172.217.0.36	https://www.google.com
    5 1554917744	19.876	12.394	56.910	73.899	2.003	145.440	200	12040	192.168.2.35	172.217.164.100	https://www.google.com
    
    Recorded 5 samples in 41s, average values:
    # timestamp	DNS	TCP	TLS	First	LastB	Total	HTTP	Size	From_Location	Remote_Addr	proto://uri
    5 41s   	9.738	14.567	68.615	64.764	2.193	151.155		12032		https://www.google.com
    
Each line has a request count (1..5), the epoch timestamp when the test started, and the time in
milliseconds measured for the following actions:
  * DNS: how long to look up the IP address(es) for the hostname
  * TCP: how long the TCP three-way handshake took to set up the connection
  * TLS: how long the SSL/TLS handshake took to establish a secure channel
  * First: how long until the first byte of the reply arrived (HTTP response headers)
  * LastB: how long until the last byte of the reply arrived (HTTP content body)
  * Total: response time of the application, from start of TCP connection until last byte
  * HTTP: response code returned from the server; 500 indicates a failure to connect
  * Size: response size in content bytes received from the upstream (response body, not headers)
  * From_Location: where you said the test was running from (REP_LOCATION environment variable)
  * Remote_Addr: the IP address hit by the test (may change over time, based upon DNS result)
  * proto://uri: the request URL (protocol and URI requested)

The final section provides the count of samples, the total time, and averages for the above values.
If you test to multiple endpoints you'll see multiple sections as each completes.

> Interestingly, in the example above we see the remote address changed in the last sample, following a
> DNS resolution.  Each test makes a DNS query; most of them return quickly from cache, but the last
> one fetched a fresh answer -- and it changed.

**Docker**: To run the containerized app you can say "gmake run" from the command line, which will
build the docker image (if needed) and run it out of the local docker repo with default arguments.
You can modify the arguments in the Makefile, or use a variant of its `docker run` invocation
directly from the shell.  You'll see similar output as above.

A recent version of `perftest` is available in the DockerHub and Rafay registries as noted below.
You can fetch it from public DockerHub to your local system via `docker pull dc4jadrafay/perftest`
if you prefer not to build from source.  Or you can just run it using a command line like the one
in the Makefile:

    docker run --rm -it -e PERFTEST_URL="https://www.google.com" -e REP_LOCATION="Your Town,US" dc4jadrafay/perftest:v1 -n=5 -d=2

This example also shows how the application picks up values from the environment.  This will be
useful as we build out the edge application.

If you want to push it to your DockerHub repo, you can `gmake push`.  This
requres the following environment variables:

``` shell
export DOCKER_USER="your docker username"
export DOCKER_EMAIL="the email address you registerd with DockerHub"
```

You will need to login to DockerHub to establish credentials:

``` shell
docker login --username=${DOCKER_USER} --email=${DOCKER_EMAIL}
```

Supply your password if/when prompted.
Now you can `gmake push` to upload the docker to DockerHub.


## Run from Docker

If you don't want to build your own local copy, but you have docker
installed, you can just run a prebuilt version from there using a command
line similar to this:

``` shell
docker run dc4jadrafay/perftest:v1 -n 5 -d 1 https://www.google.com/
```
