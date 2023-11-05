# Docker setup for govarnamd server

This repo contains the Docker setup for building a single debian-slim container with the `govarnam.so` shared lib, `varnamcli`, `varnamd-govarnam` (HTTP server) compiled with glibc.


### Running the remote image
A pre-compiled Docker image is available on the GitHub registry. This is the simplest way to run the govarnam server (`port 8123` by default).

Download [config.toml](https://raw.githubusercontent.com/varnamproject/varnamd-govarnam/master/config.toml) and edit it.

Run:
```bash
docker run -e VARNAM_LEARNINGS_DIR=/govarnam/learnings -e VARNAM_VST_DIR=/govarnam/vst \
	-v $(pwd)/config.toml:/govarnam/config.toml \
	-v $(pwd)/data/learnings/:/govarnam/learnings/ \
	-v $(pwd)/data/vst/:/govarnam/vst/ \
	-v $(pwd)/data/input/:/govarnam/input/ \
	-p 8123:8123 \
	--name govarnam .io/varnamproject/govarnam:latest
```

PS: Add the `-d` flag to `docker run` to run the server in the background.


### Building the image locally
- Download the [Dockerfile](https://github.com/varnamproject/varnamd-govarnam/blob/master/Dockerfile)
- Run `docker build -t govarnam .` to build an image named `govarnam` locally.


### Running the local image
Download [config.toml](https://raw.githubusercontent.com/varnamproject/varnamd-govarnam/master/config.toml) and edit it.

```bash
docker run -e VARNAM_LEARNINGS_DIR=/govarnam/learnings -e VARNAM_VST_DIR=/govarnam/vst \
	-v $(pwd)/config.toml:/govarnam/config.toml \
	-v $(pwd)/data/learnings/:/govarnam/learnings/ \
	-v $(pwd)/data/vst/:/govarnam/vst/ \
	-v $(pwd)/data/input/:/govarnam/input/ \
	-p 8123:8123 \
	--name govarnam govarnam
```

## Training 
The image comes with `varnamcli` that can be used for training. Training data is stored in the host mounted `./data/learnings` directory.

- Run the `govarnam` container.
- Ensure that the necessary scheme (`-s` flag) [VST file](https://github.com/varnamproject/schemes/releases) is present in the local `./data/vst` directory.
- Copy the training CSV file with words to `./data/input`, eg: `./data/input/yourfile.csv`

Then run:
```bash
docker exec govarnam varnamcli -s ml -learn-from-file /govarnam/input/yourfile.csv
```
