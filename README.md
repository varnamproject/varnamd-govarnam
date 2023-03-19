# Varnam API Server

Varnam daemon which also acts as a HTTP server. This program powers https://varnamproject.com

## Installation

You need to have [govarnam](https://github.com/varnamproject/govarnam) installed in your local system for varnamd to run.

- Clone
- Run `go get` inside cloned folder
- Use `go run .` for starting varnamd

## Usage

varnamd supports the following command line arguments:

- `p` int. Run daemon in specified port
- `max-handle-count` int. Maximum number of handles can be opened for each language
- `host` string. Host for the varnam daemon server.
- `ui` string. UI directory path. Put your index.html here.
- `enable-internal-apis` boolean. Enable internal APIs
- `enable-ssl` boolean
- `cert-file-path` string. Certificate file path
- `key-file-path` string.
- `upstream` string. Provide an upstream server
- `enable-download`. string. Comma separated language identifier for which varnamd will download words from upstream
- `sync-interval` int.
- `log-to-file` boolean. If true, logs will be written to a file
- `version`

## Hosting

Make it & run it:

```bash
make ui/embed.js
make
./restart.sh
```

Preferrably use caddy for reverse proxy:

```ruby
api.varnamproject.com varnam.subinsb.com {
  reverse_proxy :8123
  header {
    Access-Control-Allow-Origin *
    Access-Control-Allow-Credentials true
    Access-Control-Allow-Methods *
    Access-Control-Allow-Headers *
    defer
  }
}
```

## API

### Transliteration

```
https://api.varnamproject.com/tl/{langCode}/{Word}
```

Sample: `https://api.varnamproject.com/tl/ml/Malayalam`. Response:

```json
{
  "success": true,
  "error": "",
  "at": "2020-07-02 11:10:30.309343848 +0000 UTC",
  "result": [
    "മലയാളം",
    "മലയാലം",
    "മലയാ‍ളം",
    "മലായാളം",
    "മലയളം",
    "മ്മലയലം",
    "മലയാളമാ",
    "മലയാളമാദ്ധ്യമത്തിലൂ",
    "മലയാളമാദ്ധ്യമത്തിലൂടെ",
    "മലയാളമാദ്ധ്യമത്തിൽ",
    "മലയാളമാദ്ധ്യമം"
  ],
  "input": "Malayalam"
}
```

##### see server.go for supported APIs.
