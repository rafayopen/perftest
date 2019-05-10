##
# Build a minimal size docker image to run the perftest application.  Leveraging from
# https://blog.codeship.com/building-minimal-docker-containers-for-go-applications/
# Must have already built perftest.exe (as a linux binary) and have ca-certificates.pem local
##

FROM scratch

ADD ca-certificates.pem /etc/ssl/certs/
ADD perftest.exe /perftest

ENTRYPOINT [ "/perftest" ]
