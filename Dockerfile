# This is a multi-stage Docker build that uses the golang:1.21-bullseye image to
# a) To compile govarnam the shared .so lib and the cli bin
# b) To compile varnamd-govarnam HTTP server that depends on the lib
# c) To copy the results into a debian:bullseye-slim image with glibc

###### Build stage
FROM golang:1.21-bullseye AS build

WORKDIR /app

# Install dependencies for git and other utilities
RUN apt-get update && apt-get install -y --no-install-recommends \
    libc-dev gcc git pkg-config sqlite3 && \
    rm -rf /var/lib/apt/lists/*

# Download and compile the shared libgovarnam.so lib
RUN git clone https://github.com/varnamproject/govarnam.git
RUN cd govarnam && go build -tags "fts5" -buildmode=c-shared -o libgovarnam.so

# Install the lib.
RUN mkdir -p /usr/local/include /usr/local/lib/pkgconfig
RUN cp govarnam/libgovarnam.so /usr/local/lib/ \
    && cp -R govarnam/c-shared* /usr/local/include/ \
    && cp govarnam/libgovarnam.h /usr/local/include/ \
    && sed "s#@INSTALL_PREFIX@#/usr/local#g" govarnam/govarnam.pc.in > /usr/local/lib/pkgconfig/govarnam.pc

# Build varnamcli.
RUN cd govarnam && go build -o varnamcli -ldflags "-s -w" ./cli

# Download and compile the varnamd HTTP server.
RUN git clone https://github.com/varnamproject/varnamd-govarnam.git
RUN cd varnamd-govarnam && go build -o varnamd-govarnam



###### Runtime stage
FROM debian:bullseye-slim

# Copy the deps and the binaries from the build stage.
COPY --from=build /usr/local/lib/libgovarnam.so /usr/local/lib/
COPY --from=build /usr/local/include/c-shared* /usr/local/include/
COPY --from=build /usr/local/include/libgovarnam.h /usr/local/include/
COPY --from=build /usr/local/lib/pkgconfig/govarnam.pc /usr/local/lib/pkgconfig/

# Binaries.
RUN mkdir -p /govarnam/ui
COPY --from=build /app/govarnam/varnamcli /usr/local/bin/
COPY --from=build /app/varnamd-govarnam/varnamd-govarnam /usr/local/bin/
COPY --from=build /app/varnamd-govarnam/ui /govarnam/ui/

# Setup the deps.
ENV LD_LIBRARY_PATH=/usr/local/lib
RUN apt-get update && apt-get install -y --no-install-recommends libc-dev sqlite3 && \
    ldconfig /usr/local/lib && \
    apt-get remove --purge -y libc-dev && \
    apt-get autoremove -y && \
    rm -rf /var/lib/apt/lists/*

EXPOSE 8123

WORKDIR /govarnam
ENTRYPOINT ["/usr/local/bin/varnamd-govarnam"]
CMD ["--config", "/govarnam/config.toml"]
