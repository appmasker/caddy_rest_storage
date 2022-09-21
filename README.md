# Caddy REST Storage

This is a prototype to use a REST server as a storage back-end for Caddy.

## Config
This storage module accepts two *required* strings: `endpoint` and `api_key`.

## Endpoint
In addition your `endpoint`, the following paths must be handled by your API:

| Path      | Method |
| ----------- | ----------- |
| `/lock`      | `POST`       |
| `/unlock`   | `POST`        |
| `/store`   | `POST`        |
| `/load`   | `POST`        |
| `/delete`   | `DELETE`        |
| `/exists`   | `POST`        |
| `/list`   | `POST`        |
| `/stat`   | `POST`        |

## API Key
A hard-coded `x-api-key` header is sent to your endpoint. Use an auth token as the value (defined by `api_key`) to authenticate the request.

## Example Config
```json
  "storage": {
    "module": "rest",
    "endpoint": "https://myapi.com/handle-tls-storage-methods",
    "api_key": "VERY-SECURE-API-KEY"
  }
```