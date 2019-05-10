# docker
DOCKER ?= docker
export DOCKER

CWD := $(shell basename ${PWD})
# Docker image name, based on current working directory
# Also the local binary name if you "go build perftest.go"
IMAGE := ${CWD}
# Version (for docker push)
VERSION := v1

# Linux build image name (does not conflict with go build)
LINUX_EXE := ${IMAGE}.exe
# List of docker images
IMAGE_LIST := ${IMAGE}-images.out

test:
	@-echo ${DOCKER} ${IMAGE}:${VERSION} ${LINUX_EXE}

##
# Supply default options to docker build
#$
define docker-build
$(DOCKER) build --rm -q
endef

##
# build the standalone perftest application
##
.PHONE: standalone
standalone:
	go build -v

.PHONY: build docker full
build docker: ${IMAGE}
${IMAGE_LIST}:	${LINUX_EXE}
	$(docker-build) -t ${IMAGE} .
	${DOCKER} images | egrep '^perftest ' > ${IMAGE_LIST}
	@-test -s ${IMAGE_LIST} || rm -f ${IMAGE_LIST}

full:	clean docker run

${LINUX_EXE}: ca-certificates.pem Dockerfile perftest.go util/*.go
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o $@ .

##
# Where to find certificates ... you may have to change this for your system
ca-certificates.pem: /usr/local/etc/openssl/cert.pem
	cp $? $@

.PHONY: run push
run:	${IMAGE_LIST}
	$(DOCKER) run --rm -it -e PERFTEST_URL="https://www.google.com" -e REP_CITY="Sunnyvale" -e REP_COUNTRY="US" -e AWS_REGION="us-west-2" -e AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID}" -e AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY}" ${IMAGE} -n=5 -d=2

push:	${IMAGE_LIST}
	docker tag perftest "${DOCKER_USER}/${IMAGE}:${VERSION}"
	docker push ${DOCKER_USER}/perftest

.PHONY: clean
clean:
	-rm -rf ${IMAGE} ${LINUX_EXE} ${IMAGE_LIST} ca-certificates.pem 
	-$(DOCKER) rmi ${IMAGE}
