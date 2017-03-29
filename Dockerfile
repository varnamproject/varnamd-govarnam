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
ENV GOPATH="${HOME}/gopath"

# making the go path
RUN mkdir "${HOME}/gopath"

# Installing libvarnam
RUN wget http://download.savannah.gnu.org/releases/varnamproject/libvarnam/source/libvarnam-3.2.5.tar.gz && \
		tar -xvzf libvarnam-3.2.5.tar.gz && \
		cd libvarnam-3.2.5 && \
		cmake . && make && make install

# For making go working with alpine muscl
RUN mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2

RUN apk add git
ENV PKG_CONFIG_PATH="/usr/local/lib/pkgconfig"
RUN go get github.com/varnamproject/varnamd

# Clean the cache for saving space
RUN apk del build-base gcc cmake cmake-doc 
RUN apk del curl wget bash build-base gcc
RUN apk del cmake cmake-doc
RUN rm -rf /var/cache/apk/*
RUN rm libvarnam-3.2.5.tar.gz
RUN rm -rf libvarnam-3.2.5
