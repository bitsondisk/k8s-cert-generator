# This Dockerfile is lifted from the Dockerfiles in golang.org/x/build, for
# example at golang.org/x/build/maintner/maintnerd.

# See https://github.com/golang/go/issues/23705
FROM debian:jessie

LABEL maintainer "ops@freenome.com"

RUN apt-get update && apt-get install -y \
	--no-install-recommends \
	ca-certificates \
	git-core \
	openssh-client \
	gnupg \
	&& rm -rf /var/lib/apt/lists/*

ENV TINI_VERSION v0.18.0
ADD https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini /tini
ADD https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini.asc /tini.asc
RUN gpg --keyserver hkp://p80.pool.sks-keyservers.net:80 --recv-keys 595E85A6B1B4779EA4DAAEC70B588DFF0527A9B7 \
 && gpg --verify /tini.asc
RUN chmod +x /tini

COPY k8s-cert-generator /

EXPOSE 8442
EXPOSE 8443

ENTRYPOINT ["/tini", "--", "/k8s-cert-generator", "--email", "ops@freenome.com"]
