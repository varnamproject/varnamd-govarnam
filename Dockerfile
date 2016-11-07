FROM alpine:3.2
MAINTAINER Navaneeth K N <navaneethkn@gmail.com>

# System upgrades and useful utilities
RUN apk update && apk upgrade
RUN apk add curl wget bash build-base gcc
RUN apk add cmake cmake-doc

# Go for compiling varnamd
RUN wget https://storage.googleapis.com/golang/go1.7.3.linux-amd64.tar.gz && tar -C /usr/local -xzf go1.7.3.linux-amd64.tar.gz
RUN rm go1.7.3.linux-amd64.tar.gz
ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="~/gopath"

# Installing libvarnam
RUN wget http://download.savannah.gnu.org/releases/varnamproject/libvarnam/source/libvarnam-3.2.5.tar.gz && \
		tar -xvzf libvarnam-3.2.5.tar.gz && \
		cd libvarnam-3.2.5 && \
		cmake . && make && make install

# Clean the cache for saving space
RUN rm -rf /var/cache/apk/*
RUN apk del build-base gcc cmake cmake-doc 
RUN rm libvarnam-3.2.5.tar.gz
RUN rm -rf libvarnam-3.2.5
