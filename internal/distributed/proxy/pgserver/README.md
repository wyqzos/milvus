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
Connect to real Milvus, INSERT and vector Search working via psql.

- [x] Wire protocol (connect, auth, query, disconnect)
- [x] `SELECT 1`, `SELECT version()`
- [x] Manual SQL parser for SELECT (vector search) and INSERT
- [x] Translator wired into conn.go
- [ ] Swap local types → real `milvuspb`/`schemapb` proto types
- [ ] Build proper `milvuspb.SearchRequest` (PlaceholderGroup, SearchParams)
- [ ] Parse `milvuspb.SearchResults` (column-oriented FieldsData → rows)
- [ ] Build proper `milvuspb.InsertRequest` (rows → column-oriented FieldData)
- [ ] Register pgserver as listener in proxy `service.go` (minimal, hardcoded port)
- [ ] **Verify:** create collection via gRPC, INSERT via psql, SELECT vector search via psql

#### Phase 2: Core DML + Core DDL
Expand query support and add basic schema operations.

- [ ] `SHOW TABLES` → `ShowCollections`
- [ ] `SELECT * FROM collection WHERE ...` → `Query` (filtered, no vector search)
- [ ] `DELETE FROM collection WHERE ...` → `Delete`
- [ ] `CREATE TABLE` → `CreateCollection` (parse column defs, map types to FieldSchema)
- [ ] `DROP TABLE [IF EXISTS]` → `DropCollection`
- [ ] `CREATE INDEX` → `CreateIndex` (map HNSW/IVFFlat)
- [ ] **Verify:** full lifecycle via psql — create table, create index, insert, search, delete, drop

#### Phase 3: SQL Parser
Replace manual parsing with proper SQL parser for correctness and full syntax support.

- [ ] Add `pg_query_go` dependency
- [ ] Rewrite parseSelect, parseInsert, parseDelete to use AST
- [ ] Remove manual string parsing helpers and `getStatementType()` hack
- [ ] Handle edge cases: aliases, quoted identifiers, expressions
- [ ] **Verify:** existing end-to-end tests still pass with new parser

#### Phase 4: Full DDL + Full DML
Complete SQL coverage.

- [ ] `DROP INDEX` → `DropIndex`
- [ ] `ALTER TABLE` → collection schema changes
- [ ] `DESCRIBE` / `\d` → `DescribeCollection`
- [ ] `UPDATE` → `Upsert`
- [ ] `INSERT ... ON CONFLICT` → `Upsert`
- [ ] **Verify:** all SQL operations work end-to-end via psql

#### Phase 5: Production Readiness
Polish for real-world use.

- [ ] Configuration via `milvus.yaml` (enable/disable, port, auth mode)
- [ ] Milvus auth passthrough
- [ ] TLS support
- [ ] Graceful shutdown
- [ ] Prepared statements (Parse/Bind/Execute)
- [ ] Handle SET commands and psql introspection queries
- [ ] Replace standard `log` with Milvus `pkg/log`
- [ ] **Verify:** psql interactive session works smoothly end-to-end

## Dependencies

- Go 1.24+
- No external Go dependencies (standard library only)
- Client: any PostgreSQL client (`brew install libpq` for psql)

## Development Notes

- Test server runs on port 15432 to avoid conflicts with real PostgreSQL (5432).
- See `CLAUDE.md` in this directory for implementation notes and temporary hacks to track.
