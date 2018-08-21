# Copyright 2017 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

MUTABLE_VERSION ?= latest
VERSION ?= $(shell git rev-parse --short HEAD)

IMAGE_NAME := gcr.io/freenome-build/k8s-cert-generator

DOCKER_IMAGE_build0=build0/k8s-cert-generator:latest
DOCKER_CTR_build0=k8s-cert-generator-build0

build0: *.go Dockerfile.0
	docker build --force-rm -f Dockerfile.0 --tag=$(DOCKER_IMAGE_build0) .

k8s-cert-generator: build0
	docker create --name $(DOCKER_CTR_build0) $(DOCKER_IMAGE_build0)
	docker cp $(DOCKER_CTR_build0):/go/bin/$@ $@
	docker rm $(DOCKER_CTR_build0)

docker-prod: k8s-cert-generator
	docker build --force-rm --tag=$(IMAGE_NAME):$(VERSION) .
	docker tag $(IMAGE_PROD):$(VERSION)

test:
	go test ./...
