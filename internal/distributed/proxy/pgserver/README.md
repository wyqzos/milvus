# pgserver - PostgreSQL Interface for Milvus

A PostgreSQL wire protocol compatible server that allows users to interact with Milvus using standard PostgreSQL clients and SQL syntax.

## Overview

This package implements the [PostgreSQL wire protocol](https://www.postgresql.org/docs/current/protocol.html), enabling any PostgreSQL-compatible client (psql, DBeaver, pgAdmin, psycopg2, node-postgres, etc.) to connect to Milvus and perform vector database operations using familiar SQL syntax.

### SQL Mapping Examples

```sql
-- Vector similarity search
SELECT id, title FROM articles
ORDER BY embedding <-> '[0.1, 0.2, ...]'
LIMIT 10;

-- Filtered query
SELECT * FROM products WHERE category = 'electronics' LIMIT 20;

-- Create a collection
CREATE TABLE my_collection (
    id BIGINT PRIMARY KEY,
    title VARCHAR(256),
    embedding VECTOR(128)
);

-- Insert data
INSERT INTO my_collection (id, title, embedding)
VALUES (1, 'hello', '[0.1, 0.2, ...]');
```

## Quick Start

```bash
# Build
cd ~/Documents/milvus
go build ./internal/distributed/proxy/pgserver/...

# Run standalone test server
go run ./internal/distributed/proxy/pgserver/cmd/

# Connect with psql (in another terminal)
psql -h localhost -p 15432 -U test

# One-shot query
psql -h localhost -p 15432 -U test -c "SELECT 1;"
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
└──────────────────┬──────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────┐
│ proxy.ProxyComponent                     │
│ (existing Milvus execution engine)       │
└─────────────────────────────────────────┘
```

## File Structure

```
pgserver/
├── server.go           # Server lifecycle (Start/Stop/Accept)
├── config.go           # Configuration (port, auth, timeouts)
├── conn.go             # Per-client connection handler + query routing
├── protocol.go         # Wire protocol read/write functions
├── messages.go         # Protocol message types and constants
├── auth.go             # Authentication (trust/md5/scram)
├── result.go           # Milvus response → PostgreSQL row formatting
├── cmd/
│   └── main.go         # Standalone test server
└── translator/
    ├── translator.go   # SQL → Milvus operation dispatcher
    ├── select.go       # SELECT → Milvus Query/Search
    ├── insert.go       # INSERT → Milvus Insert
    ├── delete.go       # DELETE → Milvus Delete
    ├── ddl.go          # CREATE/DROP TABLE → Collection operations
    └── types.go        # PostgreSQL ↔ Milvus type mapping
```

## Current Status

### Working

- [x] PostgreSQL wire protocol (connect, query, disconnect)
- [x] Startup handshake (SSL, auth, parameter status)
- [x] Trust authentication
- [x] `SELECT 1;` returns data
- [x] `SELECT version();` returns server info
- [x] RowDescription, DataRow, CommandComplete messages
- [x] Error response handling

### Roadmap

#### Phase 1: End-to-End MVP
Connect to real Milvus and get first queries working with manual SQL parsing.

- [ ] Connect to Milvus `ProxyComponent` (replace placeholder interface)
- [ ] `INSERT INTO` → Milvus Insert (manual parsing)
- [ ] `SELECT ... ORDER BY vec <-> '[...]'` → Milvus Search (manual parsing)
- [ ] `SHOW TABLES` → list Milvus collections

#### Phase 2: Core DML
Expand query support with more data operations.

- [ ] `SELECT * FROM collection WHERE ...` → Milvus Query
- [ ] `DELETE FROM` → Milvus Delete

#### Phase 3: SQL Parser
Replace manual parsing with proper SQL parser for correctness and full syntax support.

- [ ] SQL parser integration (`pg_query_go`)
- [ ] Rewrite translators to use AST instead of string matching

#### Phase 4: DDL
Schema operations.

- [ ] `CREATE TABLE` → CreateCollection
- [ ] `DROP TABLE` → DropCollection
- [ ] `CREATE INDEX` → CreateIndex

#### Phase 5: Production Readiness
Polish for real-world use.

- [ ] Register pgserver in Milvus proxy `service.go`
- [ ] Configuration via `milvus.yaml`
- [ ] Milvus auth passthrough
- [ ] TLS support
- [ ] Graceful shutdown
- [ ] Prepared statements (Parse/Bind/Execute)

## Dependencies

- Go 1.24+
- No external Go dependencies (standard library only)
- Client: any PostgreSQL client (`brew install libpq` for psql)

## Development Notes

- Test server runs on port 15432 to avoid conflicts with real PostgreSQL (5432).

### Temporary Hacks to Remove Later

- `getStatementType()` in `conn.go` - Parses SQL statement type by word-splitting (e.g., "DROP TABLE", "INSERT INTO"). This is fragile and should be replaced by proper AST node types once `pg_query_go` SQL parser is integrated.

### Temporary Standard Library Replacements

To allow standalone development and avoid Milvus multi-module dependency issues, the following Milvus packages were replaced with standard library equivalents. These should be restored when integrating into the full Milvus build.

| Current (standard library) | Replace with (Milvus) | Files affected |
|---|---|---|
| `"log"` (standard) | `"github.com/milvus-io/milvus/pkg/log"` (zap-based) | server.go, conn.go |
| `ProxyComponent` (local interface) | `types.ProxyComponent` from `"github.com/milvus-io/milvus/internal/types"` | server.go, conn.go |
| `MilvusDataType` (local constants) | `schemapb.DataType` from `"github.com/milvus-io/milvus-proto/go-api/v2/schemapb"` | translator/types.go |
| N/A (stubbed out) | `"github.com/milvus-io/milvus-proto/go-api/v2/milvuspb"` for request/response types | translator/select.go, insert.go, delete.go, ddl.go |
