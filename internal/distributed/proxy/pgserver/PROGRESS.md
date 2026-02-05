# PostgreSQL Interface for Milvus - Progress

## Branch
`feature/postgresql-interface` on fork `wyqzos/milvus`

## Status: Wire Protocol MVP Working

### Completed

- [x] Fork synced with upstream `milvus-io/milvus`
- [x] Feature branch created
- [x] Skeleton files (14 files, ~1500 lines)
- [x] Wire protocol implementation
  - [x] Startup message parsing (SSL handling, parameter extraction)
  - [x] Authentication (trust mode)
  - [x] ParameterStatus messages (server_version, encoding, etc.)
  - [x] BackendKeyData
  - [x] ReadyForQuery
  - [x] Query message parsing
  - [x] RowDescription + DataRow response
  - [x] CommandComplete
  - [x] ErrorResponse
  - [x] EmptyQueryResponse
- [x] `psql` can connect and run queries
- [x] `SELECT 1;` returns actual data
- [x] `SELECT version();` returns "PostgreSQL 15.0 (Milvus PostgreSQL Interface)"

### Not Yet Implemented

- [ ] Graceful shutdown (server hangs when client is connected)
- [ ] `SELECT 1 + 1` and other expressions
- [ ] SQL parser integration (pg_query_go or similar)
- [ ] SQL → Milvus translation layer
  - [ ] `SELECT * FROM collection` → Milvus Query
  - [ ] `SELECT ... ORDER BY vec <-> '[...]'` → Milvus Search
  - [ ] `INSERT INTO` → Milvus Insert
  - [ ] `DELETE FROM` → Milvus Delete
  - [ ] `CREATE TABLE` → Milvus CreateCollection
  - [ ] `DROP TABLE` → Milvus DropCollection
  - [ ] `SHOW TABLES` → List collections
- [ ] Connect translator to Milvus ProxyComponent (replace placeholder interface)
- [ ] Milvus auth passthrough (use Milvus credentials instead of trust mode)
- [ ] TLS support
- [ ] Prepared statements (Parse/Bind/Execute messages)
- [ ] Register pgserver in Milvus proxy service.go
- [ ] Configuration via milvus.yaml

## File Structure

```
internal/distributed/proxy/pgserver/
├── server.go           # Server lifecycle (Start/Stop/Accept)
├── config.go           # Configuration struct
├── conn.go             # Per-client connection handler + query handling
├── protocol.go         # Wire protocol read/write functions
├── messages.go         # Message type constants and structs
├── auth.go             # Authentication (trust/md5/scram stubs)
├── result.go           # Milvus response → PostgreSQL row formatting
├── cmd/
│   └── main.go         # Standalone test server (port 15432)
└── translator/
    ├── translator.go   # Main SQL parser/executor
    ├── select.go       # SELECT → Query/Search
    ├── insert.go       # INSERT → Insert
    ├── delete.go       # DELETE → Delete
    ├── ddl.go          # CREATE/DROP TABLE → Collection ops
    └── types.go        # SQL ↔ Milvus type mapping
```

## Architecture

```
psql / any PostgreSQL client
        │
        │ TCP (port 15432)
        ▼
┌─────────────────────────────────────────┐
│ server.go  →  conn.go  →  protocol.go  │
│ (accept)     (handle)     (parse/write) │
└──────────────────┬──────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────┐
│ translator/                              │
│ SQL → Milvus request translation         │
│ (NOT YET CONNECTED)                      │
└──────────────────┬──────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────┐
│ proxy.ProxyComponent                     │
│ (existing Milvus - NOT YET CONNECTED)    │
└─────────────────────────────────────────┘
```

## How to Run

```bash
# Build
cd ~/Documents/milvus
go build ./internal/distributed/proxy/pgserver/...

# Run test server
go run ./internal/distributed/proxy/pgserver/cmd/

# Connect (in another terminal)
psql -h localhost -p 15432 -U test

# One-shot query
psql -h localhost -p 15432 -U test -c "SELECT 1;"
```

## Next Steps (Recommended Order)

1. **Fix graceful shutdown** - Close connections on Ctrl+C
2. **Add SHOW TABLES** - First query that talks to Milvus
3. **Add SQL parser** - `go get github.com/pganalyze/pg_query_go/v5`
4. **Connect to Milvus ProxyComponent** - Replace placeholder interface
5. **Implement SELECT → Query** - Real data from Milvus collections
6. **Implement vector search** - `ORDER BY embedding <-> '[...]'`

## Dependencies

- Go 1.24+ (installed)
- libpq / psql client (`brew install libpq`)
- No external Go dependencies yet (all standard library)

## Notes

- Using standard library `log` instead of Milvus `pkg/log` to avoid module issues
- Using local `ProxyComponent` interface instead of `types.ProxyComponent`
- These should be swapped when integrating into the real Milvus build
- Server version is set to "15.0" - modern enough for client compatibility
- Port 15432 used to avoid conflicts with real PostgreSQL (5432)
