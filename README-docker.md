# Docker setup

This repo contains the Docker setup for building a single debian-slim container with the `libgovarnam.so` shared lib, `varnamcli`, `varnamd-govarnam` (HTTP server) compiled with glibc.

### Running the remote image

A pre-compiled Docker image is available on the GitHub registry. This is the simplest way to run the server (`port 8123` by default).

Download [config.toml](https://raw.githubusercontent.com/varnamproject/varnamd-govarnam/master/config.toml) and edit it.

Run:

```bash
docker run -e VARNAM_LEARNINGS_DIR=/varnamd/learnings -e VARNAM_VST_DIR=/varnamd/vst \
	-v $(pwd)/config.toml:/varnamd/config.toml \
	-v $(pwd)/data/learnings/:/varnamd/learnings/ \
	-v $(pwd)/data/vst/:/varnamd/vst/ \
	-v $(pwd)/data/input/:/varnamd/input/ \
	-p 8123:8123 \
	--name varnamd varnamproject.github.io/varnamproject/varnamd:latest
```

PS: Add the `-d` flag to `docker run` to run the server in the background.

### Building the image locally

- Download the [Dockerfile](https://github.com/varnamproject/varnamd-govarnam/blob/master/Dockerfile)
- Run `docker build -t varnamd .` to build an image named `varnamd` locally.

### Running the local image

Download [config.toml](https://raw.githubusercontent.com/varnamproject/varnamd-govarnam/master/config.toml) and edit it.

```bash
docker run -e VARNAM_LEARNINGS_DIR=/varnamd/learnings -e VARNAM_VST_DIR=/varnamd/vst \
	-v $(pwd)/config.toml:/varnamd/config.toml \
	-v $(pwd)/data/learnings/:/varnamd/learnings/ \
	-v $(pwd)/data/vst/:/varnamd/vst/ \
	-v $(pwd)/data/input/:/varnamd/input/ \
	-p 8123:8123 \
	--name varnamd varnamd
```

## Importing words

[Read this doc](https://varnamproject.com/docs/learning/) to learn more on teaching words to Varnam.

The image comes with `varnamcli` that can be used for training. Varnam dictionary is stored in the host mounted `./data/learnings` directory.

- Run the `varnamd` container.
- Ensure that the necessary scheme (`-s` flag) [VST file](https://github.com/varnamproject/schemes/releases) is present in the local `./data/vst` directory.
- Copy the files needed for learning to `./data/input`, eg: `./data/input/yourfile.txt`

Then run:

```bash
docker exec varnamd varnamcli -s ml -learn-from-file /govarnam/input/yourfile.txt
```
