// MIT License
//
// Copyright 2018 Canonical Ledgers, LLC
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
// IN THE SOFTWARE.

// Package contract provides functions and SQL framents for working with the
// "contract" table, which stores Wasm contract data indexed by its data-store
// Chain ID.
package contract

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"

	"crawshaw.io/sqlite"
	"crawshaw.io/sqlite/sqlitex"
	"github.com/Factom-Asset-Tokens/factom"
	"github.com/Factom-Asset-Tokens/factom/fat104"
	"github.com/Factom-Asset-Tokens/factom/fat107"
	"github.com/wasmerio/go-ext-wasm/wasmer"
)

// CreateTable is a SQL string that creates the "contract" table.
//
// The "contract" table stores the raw Wasm smart contract code as well as
// cached compiled modules with metering code injected.
const CreateTable = `CREATE TABLE IF NOT EXISTS "contract"."contract" (
        "id"            INTEGER PRIMARY KEY,
        "chainid"       BLOB NOT NULL UNIQUE,
        "valid"         BOOL NOT NULL DEFAULT TRUE,
        "first_entry"   BLOB NOT NULL,
        "abi"           TEXT CONSTRAINT "invalid json" CHECK (
                                ("abi" == NULL) OR
                                json_valid("abi")
                        ),
        "wasm"          BLOB,
        "compiled"      BLOB
);
CREATE INDEX IF NOT EXISTS
        "contract"."idx_contract_chainid" ON "contract"("chainid");
`

// Insert the wasm contract into the "contract" table.
//
// The first factom.Entry is the data store chain first entry. This is used to
// determine the ChainID and also is stored so that the hash of the wasm may be
// verified later.
//
// If compiled is nil, then the contract is set to be globally invalid.
func Insert(conn *sqlite.Conn, con fat104.Contract, compiled []byte) (int64, error) {
	data, err := con.Entry.MarshalBinary()
	if err != nil {
		panic(fmt.Errorf("factom.Entry.MarshalBinary(): %w", err))
	}

	stmt := conn.Prep(`INSERT INTO "contract"."contract"
                ("chainid", "valid", "first_entry", "wasm", "compiled", "abi")
                VALUES (?, ?, ?, ?, ?, json_extract(?, '$.abi'));`)
	stmt.BindBytes(1, con.Entry.ChainID[:])
	stmt.BindBool(2, len(compiled) > 0)
	stmt.BindBytes(3, data)
	stmt.BindBytes(4, con.Wasm)
	if len(compiled) > 0 {
		stmt.BindBytes(5, compiled)
		stmt.BindBytes(6, con.Entry.Content)
	} else {
		stmt.BindNull(5)
		stmt.BindNull(6)
	}

	if _, err := stmt.Step(); err != nil {
		return -1, err
	}
	return conn.LastInsertRowID(), nil
}

// SelectWhere is a SQL fragment for retrieving rows from the "contract" table
// with Select().
const SelectWhere = `SELECT "id", "valid", "compiled", "wasm"
                                FROM "contract"."contract" WHERE `
const (
	colID = iota
	colValid
	colCompiled
	colWasm
)

// SelectModule returns the compiled wasmer.Module from the given prepared
// Stmt, which must be prepared on SQL that starts with SelectWhere.
//
// An unknown contract will return (nil, -1, nil).
//
// A known invalid contract will return (nil, rowid, nil).
//
// A valid contract will return the compiled wasmer.Module, the rowid, and nil
// error.
//
// The Stmt must be created with a SQL string starting with SelectWhere.
func SelectModule(stmt *sqlite.Stmt) (*wasmer.Module, int64, error) {
	hasRow, err := stmt.Step()
	if err != nil || !hasRow {
		return nil, -1, err
	}

	id := stmt.ColumnInt64(colID)
	if stmt.ColumnInt32(colValid) == 0 {
		// Known, but invalid.
		return nil, id, nil
	}

	// Attempt to load pre-compiled module
	if stmt.ColumnLen(colCompiled) > 0 {
		compiled := make([]byte, stmt.ColumnLen(colCompiled))
		stmt.ColumnBytes(colCompiled, compiled)
		mod, err := wasmer.DeserializeModule(compiled)
		if err == nil {
			return &mod, id, nil
		}
	}
	// Fallback to compiling module if the cache is corrupted or
	// missing.

	wasm := make([]byte, stmt.ColumnLen(colWasm))
	stmt.ColumnBytes(colWasm, wasm)
	mod, err := wasmer.Compile(wasm)
	if err != nil {
		return nil, id, fmt.Errorf("wasmer.Compile(): %w", err)
	}

	return &mod, id, nil
}

// SelectValid returns whether the contract with the given chainID is marked valid.
//
// An unknown contract will return (false, -1, nil).
//
// A known invalid contract will return (false, rowid, nil).
//
// A known valid contract will return (false, rowid, nil).
func SelectValid(conn *sqlite.Conn, chainID *factom.Bytes32) (bool, int64, error) {
	stmt := conn.Prep(`SELECT "id", "valid" FROM "contract"."contract"
                                WHERE "chainid" = ?;`)
	stmt.BindBytes(1, chainID[:])

	hasRow, err := stmt.Step()
	if err != nil || !hasRow {
		return false, -1, err
	}

	return stmt.ColumnInt32(colValid) != 0, stmt.ColumnInt64(colID), nil
}

// SelectByID returns the compiled wasmer.Module for the contract stored at row
// id.
//
// See Select for more information on return values.
func SelectByID(conn *sqlite.Conn, id int64) (*wasmer.Module, error) {
	stmt := conn.Prep(SelectWhere + `"id" = ?;`)
	stmt.BindInt64(1, id)
	defer stmt.Reset()
	mod, _, err := SelectModule(stmt)
	return mod, err
}

// SelectABIFunc returns the fat104.Func with the given fname from the contract
// with the given conID.
//
// If no such conID exists, a "contract not found" error is returned.
//
// If no such fname exists, (nil, nil) is returned.
func SelectABIFunc(conn *sqlite.Conn, conID int64, fname string) (
	*fat104.Func, error) {

	stmt := conn.Prep(fmt.Sprintf(
		`SELECT json_extract("abi", '$.%v') FROM "contract"."contract"
                        WHERE "id" = ?;`, fname))
	stmt.BindInt64(1, conID)

	hasRow, err := stmt.Step()
	if !hasRow {
		return nil, fmt.Errorf("contract not found")
	}
	if err != nil {
		return nil, err
	}

	if stmt.ColumnType(0) == sqlite.SQLITE_NULL {
		return nil, nil
	}
	fJSON := []byte(stmt.ColumnText(0))

	var f fat104.Func
	if err := json.Unmarshal(fJSON, &f); err != nil {
		return nil, fmt.Errorf("json.Unmarshal(): %w", err)
	}

	f.Name = fname
	return &f, nil
}

// SelectByChainID returns the contract with given chainID.
//
// See Select for more information on return values.
func SelectByChainID(conn *sqlite.Conn, chainID *factom.Bytes32) (*wasmer.Module, int64, error) {
	stmt := conn.Prep(SelectWhere + `"chainid" = ?;`)
	stmt.BindBytes(1, chainID[:])
	defer stmt.Reset()
	return SelectModule(stmt)
}

// SelectCount returns the total number of rows in the "contract" table. If
// validOnly is true, only the rows where "valid" = true are counted.
func SelectCount(conn *sqlite.Conn, validOnly bool) (int64, error) {
	stmt := conn.Prep(`SELECT count(*) FROM "contract"."contract"
                WHERE (? OR "valid" = true);`)
	stmt.BindBool(1, !validOnly)
	return sqlitex.ResultInt64(stmt)
}

func SelectIsCached(conn *sqlite.Conn, id int64) (bool, error) {
	stmt := conn.Prep(`SELECT length("compiled") FROM "contract"."contract"
                WHERE "id" = ?;`)
	stmt.BindInt64(1, id)
	hasRow, err := stmt.Step()
	if err != nil {
		return false, err
	}
	if !hasRow {
		return false, fmt.Errorf("invalid id")
	}
	return stmt.ColumnInt32(0) > 0, nil
}

// Cache updates the compiled module cache for the contract at row id with the
// given mod.
func Cache(conn *sqlite.Conn, id int64, mod *wasmer.Module) error {
	compiled, err := mod.Serialize()
	if err != nil {
		return fmt.Errorf("wasmer.Module.Serialize(): %w", err)
	}
	stmt := conn.Prep(`UPDATE "contract" SET "compiled" = ? WHERE id = ?;`)
	stmt.BindBytes(1, compiled)
	stmt.BindInt64(2, id)
	defer stmt.Reset()
	_, err = stmt.Step()
	return err
}

func ClearCompiledCache(conn *sqlite.Conn) error {
	stmt := conn.Prep(`UPDATE "contract" SET "compiled" = NULL;`)
	defer stmt.Reset()
	_, err := stmt.Step()
	return err
}

// Validate the integrity of the entire contract database for all valid
// contracts by recomputing the hashes for all data.
//
// TODO: This should also re-validate the "valid" column and ABIs by
// re-assessing the validity of all contracts marked invalid.
func Validate(conn *sqlite.Conn) error {
	stmt := conn.Prep(`SELECT "id", "chainid", "first_entry" FROM "contract";`)

	var err error
	for hasRow := true; hasRow && err == nil; {
		hasRow, err = validate(conn, stmt)
	}

	return err
}

func validate(conn *sqlite.Conn, stmt *sqlite.Stmt) (bool, error) {
	hasRow, err := stmt.Step()
	if err != nil || !hasRow {
		return hasRow, err
	}

	var e factom.Entry
	e.ChainID = new(factom.Bytes32)
	if stmt.ColumnBytes(1, e.ChainID[:]) != len(e.ChainID) {
		panic("invalid ChainID length")
	}

	data := make([]byte, stmt.ColumnLen(2))
	stmt.ColumnBytes(2, data)
	if err := e.UnmarshalBinary(data); err != nil {
		return false, fmt.Errorf("factom.Entry.UnmarshalBinary(): %w", err)
	}

	if *e.ChainID != factom.ComputeChainID(e.ExtIDs) {
		return false, fmt.Errorf("first_entry ExtIDs does not match ChainID")
	}

	m, err := fat107.ParseEntry(e)
	if err != nil {
		return false, fmt.Errorf("fat107.ParseEntry(): %w", err)
	}

	id := stmt.ColumnInt64(0)
	blob, err := conn.OpenBlob("contract", "contract", "wasm", id, false)
	if err != nil {
		return false, fmt.Errorf("sqlite.Conn.OpenBlob(): %w", err)
	}
	if blob.Size() != int64(m.Size) {
		return false, fmt.Errorf("corrupted wasm blob: invalid size")
	}
	hash := sha256.New()
	block := make([]byte, hash.BlockSize())
	for {
		n, err := blob.Read(block)
		if err != nil {
			if err != io.EOF {
				return false, fmt.Errorf("sqlite.Blob.Read(): %w", err)
			}
		}
		if n == 0 {
			break
		}
		if _, err := hash.Write(block[:n]); err != nil {
			return false, fmt.Errorf("sha256.New().Write(): %w", err)
		}
	}
	if *m.DataHash != sha256.Sum256(hash.Sum(nil)) {
		return false, fmt.Errorf("corrupted wasm blob: invalid hash")
	}
	return true, nil
}