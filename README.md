# Varnam API server

A HTTP server frontend for Varnam. This program powers https://api.varnamproject.com

## Installation

- Install [Varnam](https://varnamproject.com/)
- Clone this repo and run `make`

## Docker

For Docker installation and usage, refer to the [Docker README](README-docker.md).

## Hosting

Make it & run it:

```bash
make ui/embed.js
make
./restart.sh
```

Preferrably use [Caddy](https://caddyserver.com/) for reverse proxy:

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
https://api.varnamproject.com/tl/{langCode}/{word}
```

A GET request to `https://api.varnamproject.com/tl/ml/malayalam` will give the response:

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
  "input": "malayalam"
}
```

See `server.go` for the full API list.
