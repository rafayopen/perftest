# docker
DOCKER ?= docker
export DOCKER

CWD := $(shell basename ${PWD})
# Docker image name, based on current working directory
# Also the local binary name if you "go build perftest.go"
IMAGE := ${CWD}
# Version (tag used with docker push)
VERSION := v3

# Linux build image name (does not conflict with go build)
LINUX_EXE := ${IMAGE}.exe
# List of docker images
IMAGE_LIST := ${IMAGE}-images.out

test:
	@-echo Use \"make docker\" to $(DOCKER) image ${IMAGE}:${VERSION} from ${LINUX_EXE}
# "make push" will push it to DockerHub, using credentials in your env

##
# Supply default options to docker build
#$
define docker-build
$(DOCKER) build --rm -q
endef

##
# build the standalone perftest application
##
.PHONY: standalone install
${IMAGE}: *.go */*.go
	go build -v && go test -v && go vet

standalone: ${IMAGE}
install:	${IMAGE}
	go install

.PHONY: build docker full 
build docker: ${IMAGE_LIST}
${IMAGE_LIST}:	${LINUX_EXE} Dockerfile Makefile
	$(docker-build) -t ${IMAGE} .
	$(DOCKER) tag ${IMAGE} "${IMAGE}:${VERSION}" # tag local image name with version
	$(DOCKER) images | egrep '^perftest ' > ${IMAGE_LIST}
	@-test -s ${IMAGE_LIST} || rm -f ${IMAGE_LIST}

full:	clean docker run

${LINUX_EXE}: perftest.go */*.go
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o $@ .

.PHONY: run push
run:	${IMAGE_LIST}
	$(DOCKER) run --rm -it -e PERFTEST_URL="https://www.google.com" -e REP_CITY="Sunnyvale" -e REP_COUNTRY="US" -e AWS_REGION="us-west-2" -e AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID}" -e AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY}" ${IMAGE} -n=5 -d=2

push:	${IMAGE_LIST}
	$(DOCKER) tag ${IMAGE} "${DOCKER_USER}/${IMAGE}:${VERSION}"
	$(DOCKER) push ${DOCKER_USER}/${IMAGE}

.PHONY: clean
clean:
	-rm -rf ${IMAGE} ${LINUX_EXE} ${IMAGE_LIST}
	-$(DOCKER) rmi ${IMAGE}:${VERSION}
