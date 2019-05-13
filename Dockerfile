##
# Build a docker image to run the perftest application.
# The CloudWatch version requires /bin/sh, so build from Alpine.
##
FROM alpine:3.4

RUN apk update && apk add ca-certificates && /bin/rm -rf /var/cache/apk/*
ADD perftest.exe /usr/local/bin/perftest

# Include -c option to publish to CloudWatch.  You must also set the following
# environment variables: AWS_REGION, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY
ENTRYPOINT [ "/usr/local/bin/perftest", "-c" ]
